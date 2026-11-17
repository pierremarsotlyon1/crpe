//go:build integration

package queue_test

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pierremarsotlyon1/crpe/backend/internal/database"
	"github.com/pierremarsotlyon1/crpe/backend/internal/httpapi"
	"github.com/pierremarsotlyon1/crpe/backend/internal/probe"
	"github.com/pierremarsotlyon1/crpe/backend/internal/queue"
	"github.com/pierremarsotlyon1/crpe/backend/internal/worker"
	"github.com/pierremarsotlyon1/crpe/backend/migrations"
)

func TestConcurrentWorkersClaimEachJobOnce(t *testing.T) {
	pool := testPool(t, true)
	secondPool, err := pgxpool.NewWithConfig(t.Context(), pool.Config())
	if err != nil {
		t.Fatal(err)
	}
	defer secondPool.Close()
	const count = 24
	for i := range count {
		enqueue(t, pool, fmt.Sprintf("concurrent-%d", i), 5)
	}
	queues := []*queue.Queue{queue.New(pool), queue.New(secondPool)}
	results := make(chan queue.Job, count)
	errorsFound := make(chan error, count)
	var group sync.WaitGroup
	for i := range count {
		group.Go(func() {
			job, err := queues[i%len(queues)].Claim(t.Context(), time.Minute)
			if err != nil {
				errorsFound <- err
				return
			}
			results <- job
		})
	}
	group.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Error(err)
	}
	close(results)
	seen := make(map[int64]bool)
	for job := range results {
		if seen[job.ID] {
			t.Errorf("job %d was claimed twice", job.ID)
		}
		seen[job.ID] = true
		if err := probe.Handler(pool)(t.Context(), worker.Message{ID: fmt.Sprint(job.ID)}); err != nil {
			t.Fatal(err)
		}
		if err := queues[0].Ack(t.Context(), job); err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) != count {
		t.Fatalf("claimed %d jobs, want %d", len(seen), count)
	}
	var effects int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM job_probe_results").Scan(&effects); err != nil {
		t.Fatal(err)
	}
	if effects != count {
		t.Fatalf("durable effects = %d, want %d", effects, count)
	}
}

func TestExpiredLeaseIsReclaimedAndOldWorkerIsFenced(t *testing.T) {
	pool := testPool(t, true)
	enqueue(t, pool, "recover-crash", 5)
	q := queue.New(pool)
	first, err := q.Claim(t.Context(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), "UPDATE jobs SET lease_until = now() - interval '1 second' WHERE id = $1", first.ID); err != nil {
		t.Fatal(err)
	}
	if err := q.Ack(t.Context(), first); !errors.Is(err, queue.ErrLostLease) {
		t.Fatalf("expired lease must reject completion before reclaim: %v", err)
	}
	if err := q.Fail(t.Context(), first, 0); !errors.Is(err, queue.ErrLostLease) {
		t.Fatalf("expired lease must reject retry before reclaim: %v", err)
	}
	second, err := q.Claim(t.Context(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID || second.ClaimToken == first.ClaimToken || second.Attempts != 2 {
		t.Fatalf("invalid recovery: first=%+v second=%+v", first, second)
	}
	if err := q.Ack(t.Context(), first); !errors.Is(err, queue.ErrLostLease) {
		t.Fatalf("old completion must be fenced: %v", err)
	}
	if err := q.Fail(t.Context(), first, 0); !errors.Is(err, queue.ErrLostLease) {
		t.Fatalf("old retry must be fenced: %v", err)
	}
	// A crash after effect commit but before acknowledgement may replay the handler.
	for range 2 {
		if err := probe.Handler(pool)(t.Context(), worker.Message{ID: fmt.Sprint(second.ID)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := q.Ack(t.Context(), second); err != nil {
		t.Fatal(err)
	}
	if err := q.Ack(t.Context(), second); !errors.Is(err, queue.ErrLostLease) {
		t.Fatalf("duplicate completion must not be applied: %v", err)
	}
	var effects int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM job_probe_results WHERE job_id=$1", first.ID).Scan(&effects); err != nil {
		t.Fatal(err)
	}
	if effects != 1 {
		t.Fatalf("probe must have exactly one durable effect, got %d", effects)
	}
}

func TestRetriesAreBoundedAndCrashExhaustionIsVisible(t *testing.T) {
	pool := testPool(t, true)
	q := queue.New(pool)
	enqueue(t, pool, "bounded-retry", 2)
	for attempt := 1; attempt <= 2; attempt++ {
		job, err := q.Claim(t.Context(), time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if job.Attempts != attempt {
			t.Fatalf("attempt = %d, want %d", job.Attempts, attempt)
		}
		if err := q.Fail(t.Context(), job, 0); err != nil {
			t.Fatal(err)
		}
	}
	assertNoJob(t, q)
	assertStatus(t, pool, "bounded-retry", "failed")
	enqueue(t, pool, "final-attempt-crash", 1)
	job, err := q.Claim(t.Context(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), "UPDATE jobs SET lease_until=now()-interval '1 second' WHERE id=$1", job.ID); err != nil {
		t.Fatal(err)
	}
	assertNoJob(t, q)
	assertStatus(t, pool, "final-attempt-crash", "failed")
}

func TestRetryDelayAndEnqueueDeduplication(t *testing.T) {
	pool := testPool(t, true)
	q := queue.New(pool)
	enqueue(t, pool, "same-event", 5)
	if _, err := pool.Exec(t.Context(), `INSERT INTO jobs(kind,deduplication_key)
		VALUES ('system.probe','same-event') ON CONFLICT(deduplication_key) DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	job, err := q.Claim(t.Context(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	assertNoJob(t, q)
	if err := q.Fail(t.Context(), job, time.Hour); err != nil {
		t.Fatal(err)
	}
	assertNoJob(t, q)
	assertStatus(t, pool, "same-event", "pending")
}

func TestReadinessRequiresMigrationsAndMigrationsAreRepeatable(t *testing.T) {
	pool := testPool(t, false)
	handler := httpapi.New(func(ctx context.Context) error { return database.Ready(ctx, pool) }, false)
	assertHTTPStatus(t, handler, "/health/live", 200)
	assertHTTPStatus(t, handler, "/health/ready", 503)
	applyMigrations(t, pool)
	var firstCount int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM schema_migrations").Scan(&firstCount); err != nil {
		t.Fatal(err)
	}
	applyMigrations(t, pool)
	assertHTTPStatus(t, handler, "/health/ready", 200)
	var applied int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM schema_migrations").Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if firstCount == 0 || applied != firstCount {
		t.Fatalf("migration reapplied: %d ledger entries", applied)
	}
	if _, err := pool.Exec(t.Context(), "UPDATE schema_migrations SET checksum='tampered'"); err != nil {
		t.Fatal(err)
	}
	conn, err := pool.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if err := migrations.Apply(t.Context(), conn.Conn()); err == nil {
		t.Fatal("modified migration checksum was accepted")
	}
}

func TestConcurrentMigrationsSerialize(t *testing.T) {
	pool := testPool(t, false)
	results := make(chan error, 2)
	for range 2 {
		go func() {
			conn, err := pool.Acquire(t.Context())
			if err != nil {
				results <- err
				return
			}
			defer conn.Release()
			results <- migrations.Apply(t.Context(), conn.Conn())
		}()
	}
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if err := database.Ready(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
}

func testPool(t *testing.T, migrated bool) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("integration tests require TEST_DATABASE_URL pointing at a disposable PostgreSQL database")
	}
	admin, err := pgx.Connect(t.Context(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	var random [8]byte
	_, _ = rand.Read(random[:])
	schema := fmt.Sprintf("test_%x", random)
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(t.Context(), "CREATE SCHEMA "+quotedSchema); err != nil {
		_ = admin.Close(t.Context())
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.MaxConns = 5
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+quotedSchema+" CASCADE"); err != nil {
			t.Errorf("clean test schema: %v", err)
		}
		_ = admin.Close(ctx)
	})
	if migrated {
		applyMigrations(t, pool)
	}
	return pool
}

func applyMigrations(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	conn, err := pool.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if err := migrations.Apply(t.Context(), conn.Conn()); err != nil {
		t.Fatal(err)
	}
}

func enqueue(t *testing.T, pool *pgxpool.Pool, key string, maxAttempts int) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `INSERT INTO jobs(kind,deduplication_key,max_attempts)
		VALUES ('system.probe',$1,$2)`, key, maxAttempts); err != nil {
		t.Fatal(err)
	}
}

func assertNoJob(t *testing.T, q *queue.Queue) {
	t.Helper()
	if _, err := q.Claim(t.Context(), time.Minute); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected empty queue, got %v", err)
	}
}

func assertStatus(t *testing.T, pool *pgxpool.Pool, key, expected string) {
	t.Helper()
	var status string
	if err := pool.QueryRow(t.Context(), "SELECT status FROM jobs WHERE deduplication_key=$1", key).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != expected {
		t.Fatalf("job %s has status %s, want %s", key, status, expected)
	}
}

func assertHTTPStatus(t *testing.T, handler http.Handler, path string, expected int) {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	if response.Code != expected {
		t.Fatalf("%s returned %d, want %d", path, response.Code, expected)
	}
}
