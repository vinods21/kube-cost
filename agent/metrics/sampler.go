package metrics

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/kube-cost/kube-cost/agent/inventory"
	agentv1 "github.com/kube-cost/kube-cost/proto/gen/go/cost/v1/agent"
	commonv1 "github.com/kube-cost/kube-cost/proto/gen/go/cost/v1/common"
	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Publisher interface {
	Publish(context.Context, inventory.Event) error
}

type SamplerConfig struct {
	Interval time.Duration
}

type Sampler struct {
	config    SamplerConfig
	sources   Sources
	publisher Publisher
	now       func() time.Time
}

type podContainerSpec struct {
	podUID        string
	namespace     string
	podName       string
	containerName string
	containerID   string
	requests      corev1.ResourceList
	limits        corev1.ResourceList
}

type usageSample struct {
	cpuCoreNanoseconds      *int64
	memoryWorkingSetSeconds *int64
	memoryRSSSeconds        *int64
	containerID             string
}

type summaryResult struct {
	containers map[summaryKey]usageSample
	nodes      map[string]nodeUsageSample
}

type nodeUsageSample struct {
	nodeUID                 string
	nodeName                string
	cpuCoreNanoseconds      *int64
	memoryWorkingSetSeconds *int64
	memoryRSSSeconds        *int64
}

func NewSampler(config SamplerConfig, sources Sources, publisher Publisher) *Sampler {
	if config.Interval <= 0 {
		config.Interval = 10 * time.Second
	}
	return &Sampler{
		config:    config,
		sources:   sources,
		publisher: publisher,
		now:       time.Now,
	}
}

func (s *Sampler) Start(ctx context.Context) error {
	ticker := time.NewTicker(s.config.Interval)
	defer ticker.Stop()

	if err := s.Sample(ctx); err != nil {
		slog.Error("initial metrics sample failed", "error", err)
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.Sample(ctx); err != nil {
				slog.Error("metrics sample failed", "error", err)
			}
		}
	}
}

func (s *Sampler) Sample(ctx context.Context) error {
	end := s.now().UTC()
	start := end.Add(-s.config.Interval)

	specs, err := s.specs(ctx)
	if err != nil {
		return err
	}
	summary, err := s.summary(ctx, start, end)
	if err != nil {
		slog.Warn("kubelet summary sample unavailable", "error", err)
	}
	usage, err := s.usage(ctx, start, end, summary.containers)
	if err != nil {
		return err
	}

	var published int
	for key, spec := range specs {
		sample, exists := usage[key]
		if !exists {
			continue
		}
		event := s.event(start, end, spec, sample)
		if err := s.publisher.Publish(ctx, event); err != nil {
			return err
		}
		published++
	}
	for _, sample := range summary.nodes {
		if sample.nodeUID == "" || sample.nodeName == "" || !sample.hasMeasurement() {
			continue
		}
		if err := s.publisher.Publish(ctx, s.nodeEvent(start, end, sample)); err != nil {
			return err
		}
		published++
	}
	if published == 0 {
		slog.Debug("metrics sample published no container observations")
	}
	return nil
}

func (s *Sampler) specs(ctx context.Context) (map[summaryKey]podContainerSpec, error) {
	pods, err := s.sources.Pods.List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list pods for metrics sampling: %w", err)
	}
	result := make(map[summaryKey]podContainerSpec)
	for _, pod := range pods.Items {
		if pod.UID == "" || pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		statuses := containerStatuses(pod)
		add := func(container corev1.Container) {
			status := statuses[container.Name]
			result[key(pod.Namespace, pod.Name, container.Name)] = podContainerSpec{
				podUID:        string(pod.UID),
				namespace:     pod.Namespace,
				podName:       pod.Name,
				containerName: container.Name,
				containerID:   status.ContainerID,
				requests:      container.Resources.Requests,
				limits:        container.Resources.Limits,
			}
		}
		for _, container := range pod.Spec.InitContainers {
			add(container)
		}
		for _, container := range pod.Spec.Containers {
			add(container)
		}
	}
	return result, nil
}

func (s *Sampler) usage(ctx context.Context, start, end time.Time, summary map[summaryKey]usageSample) (map[summaryKey]usageSample, error) {
	result := make(map[summaryKey]usageSample, len(summary))
	for key, value := range summary {
		result[key] = value
	}
	podMetrics, err := s.sources.Metrics.PodMetrics(ctx)
	if err != nil {
		if len(result) > 0 {
			return result, nil
		}
		return nil, fmt.Errorf("read pod metrics: %w", err)
	}
	for _, pod := range podMetrics.Items {
		windowSeconds := s.config.Interval.Seconds()
		if pod.Timestamp.Time.IsZero() {
			pod.Timestamp = metav1.NewTime(end)
		}
		for _, container := range pod.Containers {
			item := result[key(pod.Metadata.Namespace, pod.Metadata.Name, container.Name)]
			if cpu, exists := container.Usage[corev1.ResourceCPU]; exists {
				value := cpuCoreNanoseconds(cpu.MilliValue(), windowSeconds)
				item.cpuCoreNanoseconds = &value
			}
			if memory, exists := container.Usage[corev1.ResourceMemory]; exists {
				value := byteSeconds(memory.Value(), windowSeconds)
				item.memoryWorkingSetSeconds = &value
			}
			result[key(pod.Metadata.Namespace, pod.Metadata.Name, container.Name)] = item
		}
	}
	return result, nil
}

func (s *Sampler) summary(ctx context.Context, start, end time.Time) (summaryResult, error) {
	nodes, err := s.sources.Nodes.List(ctx, metav1.ListOptions{})
	if err != nil {
		return summaryResult{}, fmt.Errorf("list nodes for summary sampling: %w", err)
	}
	result := summaryResult{
		containers: make(map[summaryKey]usageSample),
		nodes:      make(map[string]nodeUsageSample),
	}
	var failed int
	for _, node := range nodes.Items {
		summary, err := s.sources.Summary.NodeSummary(ctx, node.Name)
		if err != nil {
			failed++
			continue
		}
		result.nodes[node.Name] = nodeUsage(node, summary, end.Sub(start).Seconds())
		for _, pod := range summary.Pods {
			for _, container := range pod.Containers {
				item := result.containers[key(pod.PodRef.Namespace, pod.PodRef.Name, container.Name)]
				if container.Memory != nil {
					if container.Memory.WorkingSetBytes != nil {
						value := byteSeconds(int64(*container.Memory.WorkingSetBytes), s.config.Interval.Seconds())
						item.memoryWorkingSetSeconds = &value
					}
					if container.Memory.RSSBytes != nil {
						value := byteSeconds(int64(*container.Memory.RSSBytes), s.config.Interval.Seconds())
						item.memoryRSSSeconds = &value
					}
				}
				result.containers[key(pod.PodRef.Namespace, pod.PodRef.Name, container.Name)] = item
			}
		}
	}
	if len(result.containers) == 0 && len(result.nodes) == 0 && failed > 0 {
		return summaryResult{}, fmt.Errorf("all kubelet summary requests failed")
	}
	return result, nil
}

func (s *Sampler) event(start, end time.Time, spec podContainerSpec, sample usageSample) inventory.Event {
	seconds := end.Sub(start).Seconds()
	metrics := &agentv1.ContainerMetrics{
		PodUid:                      spec.podUID,
		Namespace:                   spec.namespace,
		PodName:                     spec.podName,
		ContainerName:               spec.containerName,
		ContainerId:                 firstNonEmpty(spec.containerID, sample.containerID),
		Window:                      &commonv1.TimeWindow{Start: timestamppb.New(start), End: timestamppb.New(end)},
		CpuUsageCoreNanoseconds:     sample.cpuCoreNanoseconds,
		MemoryWorkingSetByteSeconds: sample.memoryWorkingSetSeconds,
		MemoryRssByteSeconds:        sample.memoryRSSSeconds,
		Quality:                     agentv1.MetricQuality_METRIC_QUALITY_COMPLETE,
	}
	if value, ok := cpuResourceTime(spec.requests, seconds); ok {
		metrics.CpuRequestCoreNanoseconds = &value
	}
	if value, ok := cpuResourceTime(spec.limits, seconds); ok {
		metrics.CpuLimitCoreNanoseconds = &value
	}
	if value, ok := memoryResourceTime(spec.requests, seconds); ok {
		metrics.MemoryRequestByteSeconds = &value
	}
	if value, ok := memoryResourceTime(spec.limits, seconds); ok {
		metrics.MemoryLimitByteSeconds = &value
	}
	if sample.cpuCoreNanoseconds == nil || sample.memoryWorkingSetSeconds == nil {
		metrics.Quality = agentv1.MetricQuality_METRIC_QUALITY_PARTIAL
	}
	return inventory.Event{
		Key: fmt.Sprintf("metrics/container/%s/%s/%s/%d", spec.namespace, spec.podName, spec.containerName, end.UnixNano()),
		Observation: &agentv1.Observation{
			ObservedAt:  timestamppb.New(end),
			CollectedAt: timestamppb.New(s.now().UTC()),
			Payload:     &agentv1.Observation_ContainerMetrics{ContainerMetrics: metrics},
		},
	}
}

func (s *Sampler) nodeEvent(start, end time.Time, sample nodeUsageSample) inventory.Event {
	metrics := &agentv1.NodeMetrics{
		NodeUid:                     sample.nodeUID,
		NodeName:                    sample.nodeName,
		Window:                      &commonv1.TimeWindow{Start: timestamppb.New(start), End: timestamppb.New(end)},
		CpuUsageCoreNanoseconds:     sample.cpuCoreNanoseconds,
		MemoryWorkingSetByteSeconds: sample.memoryWorkingSetSeconds,
		MemoryRssByteSeconds:        sample.memoryRSSSeconds,
		Quality:                     agentv1.MetricQuality_METRIC_QUALITY_COMPLETE,
	}
	if sample.cpuCoreNanoseconds == nil || sample.memoryWorkingSetSeconds == nil {
		metrics.Quality = agentv1.MetricQuality_METRIC_QUALITY_PARTIAL
	}
	return inventory.Event{
		Key: fmt.Sprintf("metrics/node/%s/%d", sample.nodeName, end.UnixNano()),
		Observation: &agentv1.Observation{
			ObservedAt:  timestamppb.New(end),
			CollectedAt: timestamppb.New(s.now().UTC()),
			Payload:     &agentv1.Observation_NodeMetrics{NodeMetrics: metrics},
		},
	}
}

func nodeUsage(node corev1.Node, summary *Summary, seconds float64) nodeUsageSample {
	result := nodeUsageSample{
		nodeUID:  string(node.UID),
		nodeName: node.Name,
	}
	if summary.Node.NodeName != "" {
		result.nodeName = summary.Node.NodeName
	}
	if summary.Node.CPU != nil && summary.Node.CPU.UsageNanoCores != nil {
		value := int64(float64(*summary.Node.CPU.UsageNanoCores) * seconds)
		result.cpuCoreNanoseconds = &value
	}
	if summary.Node.Memory != nil {
		if summary.Node.Memory.WorkingSetBytes != nil {
			value := byteSeconds(int64(*summary.Node.Memory.WorkingSetBytes), seconds)
			result.memoryWorkingSetSeconds = &value
		}
		if summary.Node.Memory.RSSBytes != nil {
			value := byteSeconds(int64(*summary.Node.Memory.RSSBytes), seconds)
			result.memoryRSSSeconds = &value
		}
	}
	return result
}

func (s nodeUsageSample) hasMeasurement() bool {
	return s.cpuCoreNanoseconds != nil || s.memoryWorkingSetSeconds != nil || s.memoryRSSSeconds != nil
}

func containerStatuses(pod corev1.Pod) map[string]corev1.ContainerStatus {
	result := make(map[string]corev1.ContainerStatus, len(pod.Status.ContainerStatuses)+len(pod.Status.InitContainerStatuses))
	for _, status := range pod.Status.InitContainerStatuses {
		result[status.Name] = status
	}
	for _, status := range pod.Status.ContainerStatuses {
		result[status.Name] = status
	}
	return result
}

func cpuResourceTime(resources corev1.ResourceList, seconds float64) (int64, bool) {
	quantity, exists := resources[corev1.ResourceCPU]
	if !exists {
		return 0, false
	}
	return cpuCoreNanoseconds(quantity.MilliValue(), seconds), true
}

func memoryResourceTime(resources corev1.ResourceList, seconds float64) (int64, bool) {
	quantity, exists := resources[corev1.ResourceMemory]
	if !exists {
		return 0, false
	}
	return byteSeconds(quantity.Value(), seconds), true
}

func cpuCoreNanoseconds(millicores int64, seconds float64) int64 {
	return int64(float64(millicores) * 1_000_000 * seconds)
}

func byteSeconds(bytes int64, seconds float64) int64 {
	return int64(float64(bytes) * seconds)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
