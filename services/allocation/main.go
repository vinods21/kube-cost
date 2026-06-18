package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	config, err := ConfigFromEnv()
	if err != nil {
		slog.Error("invalid allocation configuration", "error", err)
		os.Exit(1)
	}
	repository, err := OpenRepository(config.ClickHouse, config.NodeHourlyCostUSD)
	if err != nil {
		slog.Error("open allocation repository", "error", err)
		os.Exit(1)
	}
	defer repository.Close()

	server := &http.Server{
		Addr:              config.HTTPAddress,
		Handler:           NewAPI(NewEngine(repository, config.NodeHourlyCostUSD)).Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("allocation server shutdown failed", "error", err)
		}
	}()

	slog.Info("allocation service listening", "address", config.HTTPAddress)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("allocation service stopped", "error", err)
		os.Exit(1)
	}
}
