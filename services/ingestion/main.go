package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/kube-cost/kube-cost/services/ingestion/app"
)

func main() {
	if err := app.Run(context.Background(), app.ConfigFromEnv()); err != nil {
		slog.Error("ingestion service stopped", "error", err)
		os.Exit(1)
	}
}
