// Package queue coordinates durable, at-least-once jobs between stateless workers.
package queue

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrLostLease = errors.New("job lease no longer owned")

type Job struct {
	ID         int64
	Kind       string
	Payload    json.RawMessage
	ClaimToken string
	Attempts   int
}

type Queue struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Queue {
	return &Queue{pool: pool}
}

func (q *Queue) Claim(ctx context.Context, lease time.Duration) (Job, error) {
	if lease <= 0 {
		return Job{}, fmt.Errorf("lease must be positive")
	}
	// Crashed jobs that have exhausted retries must become visible dead letters.
	_, err := q.pool.Exec(ctx, `UPDATE jobs
		SET status = 'failed', claim_token = NULL, lease_until = NULL, last_error = 'lease expired after final attempt'
		WHERE status = 'running' AND lease_until <= now() AND attempts >= max_attempts`)
	if err != nil {
		return Job{}, fmt.Errorf("expire exhausted jobs: %w", err)
	}
	token := randomToken()
	var job Job
	err = q.pool.QueryRow(ctx, `WITH candidate AS (
		SELECT id FROM jobs
		WHERE attempts < max_attempts AND (
			(status = 'pending' AND available_at <= now()) OR
			(status = 'running' AND lease_until <= now())
		)
		ORDER BY available_at, id
		FOR UPDATE SKIP LOCKED LIMIT 1
	)
	UPDATE jobs AS job SET status = 'running', attempts = attempts + 1,
		claim_token = $1::uuid, lease_until = now() + $2 * interval '1 second'
	FROM candidate WHERE job.id = candidate.id
	RETURNING job.id, job.kind, job.payload, job.claim_token::text, job.attempts`,
		token, lease.Seconds(),
	).Scan(&job.ID, &job.Kind, &job.Payload, &job.ClaimToken, &job.Attempts)
	if err != nil {
		return Job{}, fmt.Errorf("claim job: %w", err)
	}
	return job, nil
}

// CompleteProbe commits a durable, idempotent side effect and acknowledgement together.
// Future external side effects need their own provider idempotency key or outbox protocol.
func (q *Queue) CompleteProbe(ctx context.Context, job Job) error {
	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin probe: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var ownedID int64
	err = tx.QueryRow(ctx, `SELECT id FROM jobs
		WHERE id = $1 AND status = 'running' AND claim_token = $2::uuid AND lease_until > now()
		FOR UPDATE`, job.ID, job.ClaimToken,
	).Scan(&ownedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrLostLease
	}
	if err != nil {
		return fmt.Errorf("check probe ownership: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO job_probe_results (job_id) VALUES ($1) ON CONFLICT DO NOTHING`, job.ID); err != nil {
		return fmt.Errorf("record probe: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE jobs SET status = 'completed', completed_at = now(),
		claim_token = NULL, lease_until = NULL, last_error = NULL WHERE id = $1`, job.ID); err != nil {
		return fmt.Errorf("acknowledge probe: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit probe: %w", err)
	}
	return nil
}

func (q *Queue) Fail(ctx context.Context, job Job, delay time.Duration) error {
	if delay < 0 {
		return fmt.Errorf("retry delay must not be negative")
	}
	result, err := q.pool.Exec(ctx, `UPDATE jobs SET
		status = CASE WHEN attempts >= max_attempts THEN 'failed' ELSE 'pending' END,
		available_at = now() + $3 * interval '1 second',
		claim_token = NULL, lease_until = NULL, last_error = 'handler failed'
		WHERE id = $1 AND status = 'running' AND claim_token = $2::uuid AND lease_until > now()`,
		job.ID, job.ClaimToken, delay.Seconds(),
	)
	if err != nil {
		return fmt.Errorf("release failed job: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrLostLease
	}
	return nil
}

func randomToken() string {
	var value [16]byte
	_, _ = rand.Read(value[:]) // crypto/rand.Read always fills the buffer or terminates the process.
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}
