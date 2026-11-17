package queue

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

// Run stops claiming on cancellation. An in-flight task receives its own bounded
// grace period; process death is recovered through the durable lease.
func (q *Queue) Run(ctx context.Context, logger *slog.Logger) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return nil
		}
		claimCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		job, err := q.Claim(claimCtx, 30*time.Second)
		cancel()
		if err == nil {
			q.process(ctx, logger, job)
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) && ctx.Err() == nil {
			logger.Error("claim job failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (q *Queue) process(ctx context.Context, logger *slog.Logger, job Job) {
	jobCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
	defer cancel()
	var err error
	switch job.Kind {
	case "system.probe":
		err = q.CompleteProbe(jobCtx, job)
	default:
		err = fmt.Errorf("unsupported job kind")
	}
	if err == nil {
		logger.Info("job completed", "job_id", job.ID, "attempt", job.Attempts)
		return
	}
	logger.Error("job failed", "job_id", job.ID, "attempt", job.Attempts, "error", err)
	retryCtx, retryCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer retryCancel()
	delay := time.Duration(min(job.Attempts*job.Attempts, 60)) * time.Second
	if err := q.Fail(retryCtx, job, delay); err != nil && !errors.Is(err, ErrLostLease) {
		logger.Error("retry scheduling failed", "job_id", job.ID, "error", err)
	}
}
