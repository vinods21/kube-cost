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
	address := os.Getenv("WORKFLOW_HTTP_ADDRESS")
	if address == "" {
		serviceentry.Run("workflow")
		return
	}
	repository, err := OpenRepository(ConfigFromEnv().ClickHouse)
	if err != nil {
		slog.Error("open workflow repository", "error", err)
		os.Exit(1)
	}
	defer repository.Close()

	server := &http.Server{
		Addr:              address,
		Handler:           NewAPI(repository, time.Now).Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("workflow API shutdown failed", "error", err)
		}
	}()
	slog.Info("workflow API listening", "address", address)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("workflow API stopped", "error", err)
		os.Exit(1)
	}
}
