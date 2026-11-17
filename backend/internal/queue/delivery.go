package queue

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pierremarsotlyon1/crpe/backend/internal/worker"
)

var _ worker.Consumer = (*Queue)(nil)

type delivery struct {
	queue *Queue
	job   Job
}

func (q *Queue) Receive(ctx context.Context) (worker.Delivery, error) {
	job, err := q.Claim(ctx, 90*time.Second)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, worker.ErrEmpty
	}
	if err != nil {
		return nil, err
	}
	return &delivery{queue: q, job: job}, nil
}

func (d *delivery) Message() worker.Message {
	return worker.Message{ID: strconv.FormatInt(d.job.ID, 10), Kind: d.job.Kind, Payload: d.job.Payload, Attempt: d.job.Attempts}
}
func (d *delivery) Ack(ctx context.Context) error { return d.queue.Ack(ctx, d.job) }
func (d *delivery) Retry(ctx context.Context, delay time.Duration) error {
	return d.queue.Fail(ctx, d.job, delay)
}

// Ack is transport-only. A handler commits its idempotent effect before this call.
func (q *Queue) Ack(ctx context.Context, job Job) error {
	result, err := q.pool.Exec(ctx, `UPDATE jobs SET status='completed', completed_at=now(),
		claim_token=NULL,lease_until=NULL,last_error=NULL
		WHERE id=$1 AND status='running' AND claim_token=$2::uuid AND lease_until>now()`, job.ID, job.ClaimToken)
	if err != nil {
		return fmt.Errorf("acknowledge job: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrLostLease
	}
	return nil
}
