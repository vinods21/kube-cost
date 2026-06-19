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
		slog.Error("invalid gateway configuration", "error", err)
		os.Exit(1)
	}
	server, err := NewServer(config)
	if err != nil {
		slog.Error("create gateway", "error", err)
		os.Exit(1)
	}
	httpServer := &http.Server{
		Addr:              config.HTTPAddress,
		Handler:           server.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("gateway shutdown failed", "error", err)
		}
	}()
	slog.Info("gateway listening", "address", config.HTTPAddress)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("gateway stopped", "error", err)
		os.Exit(1)
	}
}
