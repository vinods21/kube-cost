package controllermanager

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

type Config struct {
	Name               string
	MetricsAddress     string
	HealthProbeAddress string
	LeaderElection     bool
	LeaderElectionID   string
}

func Run(cfg Config) error {
	scheme := runtime.NewScheme()
	manager, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: cfg.MetricsAddress},
		HealthProbeBindAddress: cfg.HealthProbeAddress,
		LeaderElection:         cfg.LeaderElection,
		LeaderElectionID:       cfg.LeaderElectionID,
	})
	if err != nil {
		return fmt.Errorf("create %s manager: %w", cfg.Name, err)
	}

	if err := manager.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("add health check: %w", err)
	}
	if err := manager.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("add readiness check: %w", err)
	}

	return manager.Start(ctrl.SetupSignalHandler())
}
