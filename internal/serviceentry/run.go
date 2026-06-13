package serviceentry

import (
	"context"
	"log/slog"
	"os"

	"github.com/kube-cost/kube-cost/internal/grpcserver"
)

func Run(name string) {
	address := os.Getenv("GRPC_ADDRESS")
	if address == "" {
		address = ":8080"
	}

	if err := grpcserver.Run(context.Background(), grpcserver.Config{
		Name:    name,
		Address: address,
	}); err != nil {
		slog.Error("service stopped", "service", name, "error", err)
		os.Exit(1)
	}
}
