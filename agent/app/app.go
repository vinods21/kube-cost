package app

import (
	"context"
	"fmt"
	"net/http"

	"github.com/kube-cost/kube-cost/agent/config"
	"github.com/kube-cost/kube-cost/agent/inventory"
	agentmetrics "github.com/kube-cost/kube-cost/agent/metrics"
	"github.com/kube-cost/kube-cost/agent/transport"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

type Runtime struct {
	collector *inventory.Collector
	sampler   *agentmetrics.Sampler
	transport *transport.Client
}

func NewRuntime(collector *inventory.Collector, sampler *agentmetrics.Sampler, transportClient *transport.Client) *Runtime {
	return &Runtime{collector: collector, sampler: sampler, transport: transportClient}
}

func (r *Runtime) Start(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	transportErrors := make(chan error, 1)
	go func() {
		transportErrors <- r.transport.Run(runCtx)
	}()

	collectorErrors := make(chan error, 1)
	go func() {
		collectorErrors <- r.collector.Start(runCtx)
	}()
	samplerErrors := make(chan error, 1)
	go func() {
		samplerErrors <- r.sampler.Start(runCtx)
	}()

	select {
	case <-ctx.Done():
		return nil
	case err := <-collectorErrors:
		return err
	case err := <-samplerErrors:
		return err
	case err := <-transportErrors:
		return err
	}
}

func (r *Runtime) NeedLeaderElection() bool {
	return true
}

func Run(cfg config.Config) error {
	restConfig, err := ctrl.GetConfig()
	if err != nil {
		return fmt.Errorf("load Kubernetes configuration: %w", err)
	}
	restConfig.UserAgent = "ck-kube-agent/" + cfg.AgentVersion
	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("create Kubernetes client: %w", err)
	}

	version, err := client.Discovery().ServerVersion()
	if err != nil {
		return fmt.Errorf("discover Kubernetes version: %w", err)
	}

	buffer := transport.NewBuffer(cfg.QueueCapacity)
	collector := inventory.NewCollector(
		cfg.ClusterID,
		client,
		client.Discovery(),
		cfg.ResyncInterval,
		buffer,
	)
	metricSources, err := agentmetrics.NewSources(client, restConfig)
	if err != nil {
		return fmt.Errorf("create metrics sources: %w", err)
	}
	sampler := agentmetrics.NewSampler(agentmetrics.SamplerConfig{
		Interval: cfg.MetricsInterval,
	}, metricSources, buffer)
	transportClient := transport.NewClient(transport.ClientConfig{
		TenantID:          cfg.TenantID,
		ClusterID:         cfg.ClusterID,
		AgentInstanceID:   cfg.AgentInstanceID,
		AgentVersion:      cfg.AgentVersion,
		KubernetesVersion: version.GitVersion,
		Endpoint:          cfg.IngestionEndpoint,
		Insecure:          cfg.InsecureGRPC,
		CAFile:            cfg.TLSCAFile,
		CertFile:          cfg.TLSCertFile,
		KeyFile:           cfg.TLSKeyFile,
		ServerName:        cfg.TLSServerName,
		BatchSize:         cfg.BatchSize,
	}, buffer)

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return fmt.Errorf("register Kubernetes scheme: %w", err)
	}
	manager, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme:                        scheme,
		Metrics:                       metricsserver.Options{BindAddress: cfg.MetricsAddress},
		HealthProbeBindAddress:        cfg.HealthProbeAddress,
		LeaderElection:                cfg.LeaderElection,
		LeaderElectionID:              cfg.LeaderElectionID,
		LeaderElectionNamespace:       cfg.LeaderElectionNamespace,
		LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		return fmt.Errorf("create agent manager: %w", err)
	}

	runtimeComponent := NewRuntime(collector, sampler, transportClient)
	if err := manager.Add(runtimeComponent); err != nil {
		return fmt.Errorf("add agent runtime: %w", err)
	}
	if err := manager.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("add health check: %w", err)
	}
	if err := manager.AddReadyzCheck("inventory", func(_ *http.Request) error {
		if !collector.Ready() {
			return fmt.Errorf("inventory informers are not synchronized")
		}
		return nil
	}); err != nil {
		return fmt.Errorf("add readiness check: %w", err)
	}

	return manager.Start(ctrl.SetupSignalHandler())
}
