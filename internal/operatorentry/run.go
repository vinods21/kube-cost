package operatorentry

import (
	"log/slog"
	"os"

	"github.com/kube-cost/kube-cost/internal/controllermanager"
)

func Run(name, leaderElectionID string) {
	if configuredID := os.Getenv("LEADER_ELECTION_ID"); configuredID != "" {
		leaderElectionID = configuredID
	}

	if err := controllermanager.Run(controllermanager.Config{
		Name:               name,
		MetricsAddress:     ":8080",
		HealthProbeAddress: ":8081",
		LeaderElection:     true,
		LeaderElectionID:   leaderElectionID,
	}); err != nil {
		slog.Error("controller manager stopped", "name", name, "error", err)
		os.Exit(1)
	}
}
