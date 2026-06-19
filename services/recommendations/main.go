package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/kube-cost/kube-cost/internal/serviceentry"
)

func main() {
	if os.Getenv("TENANT_ID") == "" {
		serviceentry.Run("recommendations")
		return
	}
	config, err := ConfigFromEnv()
	if err != nil {
		slog.Error("invalid recommendations configuration", "error", err)
		os.Exit(1)
	}
	repository, err := OpenRepository(config.ClickHouse)
	if err != nil {
		slog.Error("open recommendations repository", "error", err)
		os.Exit(1)
	}
	defer repository.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	recommendations, err := NewEngine(repository, config.Engine).Recommendations(ctx, Query{
		TenantID:  config.TenantID,
		ClusterID: config.ClusterID,
	})
	if err != nil {
		slog.Error("generate recommendations", "error", err)
		os.Exit(1)
	}
	slog.Info("generated optimization recommendations", "count", len(recommendations))
	if err := repository.SaveRecommendations(ctx, recommendations); err != nil {
		slog.Error("persist optimization recommendations", "error", err)
		os.Exit(1)
	}
	slog.Info("persisted optimization recommendations", "count", len(recommendations))
	for _, recommendation := range recommendations {
		slog.Info(
			"optimization recommendation",
			"tenant_id", recommendation.TenantID,
			"cluster_id", recommendation.ClusterID,
			"scope_id", recommendation.ScopeID,
			"cpu_request_millicores", recommendation.RecommendedCPURequestMillicores,
			"memory_request_bytes", recommendation.RecommendedMemoryRequestBytes,
			"monthly_savings_usd", recommendation.EstimatedMonthlySavingsUSD,
		)
	}
	serviceentry.Run("recommendations")
}
