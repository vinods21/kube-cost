package persistence

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/kube-cost/kube-cost/services/ingestion/queue"
)

type WorkerConfig struct {
	BatchSize    int
	RetryInitial time.Duration
	RetryMaximum time.Duration
}

type Worker struct {
	config     WorkerConfig
	queue      *queue.Queue
	repository *Repository
}

func NewWorker(config WorkerConfig, batches *queue.Queue, repository *Repository) *Worker {
	if config.BatchSize <= 0 {
		config.BatchSize = 20
	}
	if config.RetryInitial <= 0 {
		config.RetryInitial = time.Second
	}
	if config.RetryMaximum <= 0 {
		config.RetryMaximum = 30 * time.Second
	}
	return &Worker{config: config, queue: batches, repository: repository}
}

func (w *Worker) Run(ctx context.Context) error {
	backoff := w.config.RetryInitial
	for {
		lease, err := w.queue.Acquire(ctx, w.config.BatchSize)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}
		if err := w.repository.Persist(ctx, lease.Items()); err != nil {
			lease.Retry()
			if errors.Is(err, ErrInvalidInventory) {
				return err
			}
			slog.Error("persist inventory batch", "error", err, "retry_after", backoff)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, w.config.RetryMaximum)
			continue
		}
		lease.Commit()
		backoff = w.config.RetryInitial
	}
}
