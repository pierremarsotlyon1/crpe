// Package probe provides a durable idempotent handler for infrastructure checks.
package probe

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pierremarsotlyon1/crpe/backend/internal/worker"
)

func Handler(pool *pgxpool.Pool) worker.Handler {
	return func(ctx context.Context, message worker.Message) error {
		id, err := strconv.ParseInt(message.ID, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid probe job ID: %w", err)
		}
		_, err = pool.Exec(ctx, `INSERT INTO job_probe_results(job_id) VALUES($1) ON CONFLICT DO NOTHING`, id)
		if err != nil {
			return fmt.Errorf("record probe effect: %w", err)
		}
		return nil
	}
}
