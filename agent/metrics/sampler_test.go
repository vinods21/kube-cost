package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/kube-cost/kube-cost/agent/inventory"
	agentv1 "github.com/kube-cost/kube-cost/proto/gen/go/cost/v1/agent"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSamplerPublishesUsageRequestsLimitsAndSummaryFields(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)
	publisher := &recordingPublisher{}
	sampler := NewSampler(SamplerConfig{Interval: 10 * time.Second}, Sources{
		Pods:  fakePods{items: []corev1.Pod{pod()}},
		Nodes: fakeNodes{items: []corev1.Node{{ObjectMeta: metav1.ObjectMeta{UID: "node-uid", Name: "node-1"}}}},
		Metrics: fakePodMetrics{metrics: &PodMetricsList{Items: []PodMetrics{{
			Metadata:  metav1.ObjectMeta{Name: "pod-1", Namespace: "apps"},
			Timestamp: metav1.NewTime(now),
			Window:    metav1.Duration{Duration: 10 * time.Second},
			Containers: []ContainerMetrics{{
				Name: "app",
				Usage: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceCPU:    resource.MustParse("250m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
			}},
		}}}},
		Summary: fakeSummary{summary: &Summary{
			Node: SummaryNode{
				NodeName: "node-1",
				CPU: &SummaryCPU{
					UsageNanoCores: uint64Pointer(1_000_000_000),
				},
				Memory: &SummaryMemory{
					WorkingSetBytes: uint64Pointer(8 * 1024 * 1024 * 1024),
					RSSBytes:        uint64Pointer(4 * 1024 * 1024 * 1024),
				},
			},
			Pods: []SummaryPod{{
				PodRef: SummaryPodRef{Name: "pod-1", Namespace: "apps", UID: "pod-uid"},
				Containers: []SummaryContainer{{
					Name: "app",
					Memory: &SummaryMemory{
						RSSBytes: uint64Pointer(64 * 1024 * 1024),
					},
				}},
			}},
		}},
	}, publisher)
	sampler.now = func() time.Time { return now }

	if err := sampler.Sample(context.Background()); err != nil {
		t.Fatal(err)
	}
	metrics := containerMetrics(t, publisher.events)
	if metrics.GetCpuUsageCoreNanoseconds() != 2_500_000_000 {
		t.Fatalf("cpu usage=%d", metrics.GetCpuUsageCoreNanoseconds())
	}
	if metrics.GetCpuRequestCoreNanoseconds() != 1_000_000_000 ||
		metrics.GetCpuLimitCoreNanoseconds() != 5_000_000_000 {
		t.Fatalf("request/limit cpu=%d/%d", metrics.GetCpuRequestCoreNanoseconds(), metrics.GetCpuLimitCoreNanoseconds())
	}
	if metrics.GetMemoryWorkingSetByteSeconds() != 128*1024*1024*10 ||
		metrics.GetMemoryRssByteSeconds() != 64*1024*1024*10 {
		t.Fatalf("memory metrics working=%d rss=%d", metrics.GetMemoryWorkingSetByteSeconds(), metrics.GetMemoryRssByteSeconds())
	}
	if metrics.GetMemoryRequestByteSeconds() != 256*1024*1024*10 ||
		metrics.GetMemoryLimitByteSeconds() != 512*1024*1024*10 {
		t.Fatalf("request/limit memory=%d/%d", metrics.GetMemoryRequestByteSeconds(), metrics.GetMemoryLimitByteSeconds())
	}
	if metrics.GetQuality() != agentv1.MetricQuality_METRIC_QUALITY_COMPLETE {
		t.Fatalf("quality=%s", metrics.GetQuality())
	}

	nodeMetrics := nodeMetrics(t, publisher.events)
	if nodeMetrics.GetCpuUsageCoreNanoseconds() != 10_000_000_000 {
		t.Fatalf("node cpu usage=%d", nodeMetrics.GetCpuUsageCoreNanoseconds())
	}
	if nodeMetrics.GetMemoryWorkingSetByteSeconds() != 8*1024*1024*1024*10 ||
		nodeMetrics.GetMemoryRssByteSeconds() != 4*1024*1024*1024*10 {
		t.Fatalf("node memory working=%d rss=%d", nodeMetrics.GetMemoryWorkingSetByteSeconds(), nodeMetrics.GetMemoryRssByteSeconds())
	}
	if nodeMetrics.GetQuality() != agentv1.MetricQuality_METRIC_QUALITY_COMPLETE {
		t.Fatalf("node quality=%s", nodeMetrics.GetQuality())
	}
}

func TestSamplerPublishesPartialSummaryFallback(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)
	publisher := &recordingPublisher{}
	sampler := NewSampler(SamplerConfig{Interval: 10 * time.Second}, Sources{
		Pods:    fakePods{items: []corev1.Pod{pod()}},
		Nodes:   fakeNodes{items: []corev1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}}}},
		Metrics: fakePodMetrics{err: context.DeadlineExceeded},
		Summary: fakeSummary{summary: &Summary{Pods: []SummaryPod{{
			PodRef: SummaryPodRef{Name: "pod-1", Namespace: "apps", UID: "pod-uid"},
			Containers: []SummaryContainer{{
				Name: "app",
				Memory: &SummaryMemory{
					WorkingSetBytes: uint64Pointer(32 * 1024 * 1024),
				},
			}},
		}}}},
	}, publisher)
	sampler.now = func() time.Time { return now }

	if err := sampler.Sample(context.Background()); err != nil {
		t.Fatal(err)
	}
	metrics := containerMetrics(t, publisher.events)
	if metrics.GetQuality() != agentv1.MetricQuality_METRIC_QUALITY_PARTIAL {
		t.Fatalf("quality=%s", metrics.GetQuality())
	}
	if metrics.CpuUsageCoreNanoseconds != nil {
		t.Fatal("CPU usage should be absent without Metrics API")
	}
	if metrics.GetMemoryWorkingSetByteSeconds() != 32*1024*1024*10 {
		t.Fatalf("summary memory fallback=%d", metrics.GetMemoryWorkingSetByteSeconds())
	}
}

func containerMetrics(t *testing.T, events []inventory.Event) *agentv1.ContainerMetrics {
	t.Helper()
	for _, event := range events {
		if metrics := event.Observation.GetContainerMetrics(); metrics != nil {
			return metrics
		}
	}
	t.Fatal("no container metrics event published")
	return nil
}

func nodeMetrics(t *testing.T, events []inventory.Event) *agentv1.NodeMetrics {
	t.Helper()
	for _, event := range events {
		if metrics := event.Observation.GetNodeMetrics(); metrics != nil {
			return metrics
		}
	}
	t.Fatal("no node metrics event published")
	return nil
}

type recordingPublisher struct {
	events []inventory.Event
}

func (p *recordingPublisher) Publish(_ context.Context, event inventory.Event) error {
	p.events = append(p.events, event)
	return nil
}

type fakePods struct{ items []corev1.Pod }

func (p fakePods) List(context.Context, metav1.ListOptions) (*corev1.PodList, error) {
	return &corev1.PodList{Items: p.items}, nil
}

type fakeNodes struct{ items []corev1.Node }

func (n fakeNodes) List(context.Context, metav1.ListOptions) (*corev1.NodeList, error) {
	return &corev1.NodeList{Items: n.items}, nil
}

type fakePodMetrics struct {
	metrics *PodMetricsList
	err     error
}

func (m fakePodMetrics) PodMetrics(context.Context) (*PodMetricsList, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.metrics, nil
}

type fakeSummary struct {
	summary *Summary
	err     error
}

func (s fakeSummary) NodeSummary(context.Context, string) (*Summary, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.summary, nil
}

func pod() corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID:       "pod-uid",
			Name:      "pod-1",
			Namespace: "apps",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:        "app",
				ContainerID: "containerd://one",
			}},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name: "app",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
				},
			}},
		},
	}
}

func uint64Pointer(value uint64) *uint64 {
	return &value
}
