package main

import (
	"log/slog"
	"os"

	"github.com/kube-cost/kube-cost/agent/app"
	"github.com/kube-cost/kube-cost/agent/config"
)

func main() {
	cfg, err := config.FromEnv()
	if err != nil {
		slog.Error("invalid agent configuration", "error", err)
		os.Exit(2)
	}
	if err := app.Run(cfg); err != nil {
		slog.Error("agent stopped", "error", err)
		os.Exit(1)
	}
}
