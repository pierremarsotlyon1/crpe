// Package migrations applies embedded, immutable SQL migrations as a one-shot task.
package migrations

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5"
)

const Latest = "0001_queue.sql"

//go:embed *.sql
var files embed.FS

func Apply(ctx context.Context, conn *pgx.Conn) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// A transaction-scoped lock serializes deploys and is automatically released on failure.
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(827463910)"); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}
	if _, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version text PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}
	names, err := fs.Glob(files, "*.sql")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	for _, name := range names {
		if err := applyFile(ctx, tx, name); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}
	return nil
}

func applyFile(ctx context.Context, tx pgx.Tx, name string) error {
	content, err := files.ReadFile(name)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", name, err)
	}
	digest := sha256.Sum256(content)
	checksum := hex.EncodeToString(digest[:])
	var appliedChecksum string
	err = tx.QueryRow(ctx, "SELECT checksum FROM schema_migrations WHERE version = $1", name).Scan(&appliedChecksum)
	if err == nil {
		if appliedChecksum != checksum {
			return fmt.Errorf("applied migration %s was modified", name)
		}
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("read migration ledger: %w", err)
	}
	if _, err := tx.Exec(ctx, string(content)); err != nil {
		return fmt.Errorf("apply migration %s: %w", name, err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version, checksum) VALUES ($1, $2)", name, checksum); err != nil {
		return fmt.Errorf("record migration %s: %w", name, err)
	}
	return nil
}
