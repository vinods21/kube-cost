package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

var schemaGroupVersionMetrics = schema.GroupVersion{Group: "metrics.k8s.io", Version: "v1beta1"}

type PodLister interface {
	List(context.Context, metav1.ListOptions) (*corev1.PodList, error)
}

type NodeLister interface {
	List(context.Context, metav1.ListOptions) (*corev1.NodeList, error)
}

type PodMetricsSource interface {
	PodMetrics(context.Context) (*PodMetricsList, error)
}

type SummarySource interface {
	NodeSummary(context.Context, string) (*Summary, error)
}

type Sources struct {
	Pods    PodLister
	Nodes   NodeLister
	Metrics PodMetricsSource
	Summary SummarySource
}

type PodMetricsList struct {
	Items []PodMetrics `json:"items"`
}

type PodMetrics struct {
	Metadata   metav1.ObjectMeta  `json:"metadata"`
	Timestamp  metav1.Time        `json:"timestamp"`
	Window     metav1.Duration    `json:"window"`
	Containers []ContainerMetrics `json:"containers"`
}

type ContainerMetrics struct {
	Name  string                                    `json:"name"`
	Usage map[corev1.ResourceName]resource.Quantity `json:"usage"`
}

type Summary struct {
	Node SummaryNode  `json:"node"`
	Pods []SummaryPod `json:"pods"`
}

type SummaryNode struct {
	NodeName string         `json:"nodeName"`
	CPU      *SummaryCPU    `json:"cpu"`
	Memory   *SummaryMemory `json:"memory"`
}

type SummaryPod struct {
	PodRef     SummaryPodRef      `json:"podRef"`
	Containers []SummaryContainer `json:"containers"`
}

type SummaryPodRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	UID       string `json:"uid"`
}

type SummaryContainer struct {
	Name      string         `json:"name"`
	StartTime metav1.Time    `json:"startTime"`
	CPU       *SummaryCPU    `json:"cpu"`
	Memory    *SummaryMemory `json:"memory"`
}

type SummaryCPU struct {
	UsageCoreNanoSeconds *uint64 `json:"usageCoreNanoSeconds"`
	UsageNanoCores       *uint64 `json:"usageNanoCores"`
}

type SummaryMemory struct {
	WorkingSetBytes *uint64 `json:"workingSetBytes"`
	RSSBytes        *uint64 `json:"rssBytes"`
}

type RESTPodMetricsSource struct {
	rest rest.Interface
}

type RESTSummarySource struct {
	rest rest.Interface
}

func NewSources(client kubernetes.Interface, restConfig *rest.Config) (Sources, error) {
	metricsREST, err := metricsRESTClient(restConfig)
	if err != nil {
		return Sources{}, err
	}
	return Sources{
		Pods:    client.CoreV1().Pods(""),
		Nodes:   client.CoreV1().Nodes(),
		Metrics: RESTPodMetricsSource{rest: metricsREST},
		Summary: RESTSummarySource{rest: client.CoreV1().RESTClient()},
	}, nil
}

func metricsRESTClient(config *rest.Config) (rest.Interface, error) {
	metricsConfig := rest.CopyConfig(config)
	metricsConfig.APIPath = "/apis"
	metricsConfig.GroupVersion = &schemaGroupVersionMetrics
	metricsConfig.NegotiatedSerializer = clientgoscheme.Codecs.WithoutConversion()
	metricsConfig.UserAgent = config.UserAgent
	client, err := rest.RESTClientFor(metricsConfig)
	if err != nil {
		return nil, fmt.Errorf("create metrics API REST client: %w", err)
	}
	return client, nil
}

func (s RESTPodMetricsSource) PodMetrics(ctx context.Context) (*PodMetricsList, error) {
	data, err := s.rest.Get().Resource("pods").DoRaw(ctx)
	if err != nil {
		return nil, fmt.Errorf("read pod metrics API: %w", err)
	}
	result := new(PodMetricsList)
	if err := json.Unmarshal(data, result); err != nil {
		return nil, fmt.Errorf("decode pod metrics API: %w", err)
	}
	return result, nil
}

func (s RESTSummarySource) NodeSummary(ctx context.Context, nodeName string) (*Summary, error) {
	path := "/api/v1/nodes/" + url.PathEscape(nodeName) + "/proxy/stats/summary"
	data, err := s.rest.Get().AbsPath(path).DoRaw(ctx)
	if err != nil {
		return nil, fmt.Errorf("read kubelet summary for node %s: %w", nodeName, err)
	}
	result := new(Summary)
	if err := json.Unmarshal(data, result); err != nil {
		return nil, fmt.Errorf("decode kubelet summary for node %s: %w", nodeName, err)
	}
	return result, nil
}

type summaryKey struct {
	namespace string
	podName   string
	container string
}

func key(namespace, podName, container string) summaryKey {
	return summaryKey{namespace: namespace, podName: podName, container: container}
}
