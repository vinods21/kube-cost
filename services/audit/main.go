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

	"github.com/kube-cost/kube-cost/internal/serviceentry"
)

func main() {
	address := os.Getenv("AUDIT_HTTP_ADDRESS")
	if address == "" {
		serviceentry.Run("audit")
		return
	}
	api := NewAPI(NewStore(1000), time.Now)
	server := &http.Server{
		Addr:              address,
		Handler:           api.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("audit API shutdown failed", "error", err)
		}
	}()
	slog.Info("audit API listening", "address", address)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("audit API stopped", "error", err)
		os.Exit(1)
	}
}
