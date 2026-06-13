package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

type Config struct {
	Name    string
	Address string
}

func Run(ctx context.Context, cfg Config) error {
	if cfg.Name == "" {
		return errors.New("service name is required")
	}
	if cfg.Address == "" {
		cfg.Address = ":8080"
	}

	listener, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Address, err)
	}

	server := grpc.NewServer()
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus(cfg.Name, healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(server, healthServer)

	runCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("gRPC service listening", "service", cfg.Name, "address", cfg.Address)
		errCh <- server.Serve(listener)
	}()

	select {
	case err := <-errCh:
		return err
	case <-runCtx.Done():
		healthServer.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
		healthServer.SetServingStatus(cfg.Name, healthpb.HealthCheckResponse_NOT_SERVING)
		stopped := make(chan struct{})
		go func() {
			server.GracefulStop()
			close(stopped)
		}()
		select {
		case <-stopped:
		case <-time.After(10 * time.Second):
			server.Stop()
		}
		return nil
	}
}
