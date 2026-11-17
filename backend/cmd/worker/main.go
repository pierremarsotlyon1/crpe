package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pierremarsotlyon1/crpe/backend/internal/config"
	"github.com/pierremarsotlyon1/crpe/backend/internal/correction"
	"github.com/pierremarsotlyon1/crpe/backend/internal/database"
	"github.com/pierremarsotlyon1/crpe/backend/internal/dialog"
	"github.com/pierremarsotlyon1/crpe/backend/internal/learning"
	"github.com/pierremarsotlyon1/crpe/backend/internal/objectstore"
	"github.com/pierremarsotlyon1/crpe/backend/internal/probe"
	"github.com/pierremarsotlyon1/crpe/backend/internal/queue"
	"github.com/pierremarsotlyon1/crpe/backend/internal/storeconfig"
	"github.com/pierremarsotlyon1/crpe/backend/internal/worker"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := database.Open(ctx, cfg.DatabaseURL, cfg.DatabaseMaxConns)
	if err != nil {
		return err
	}
	defer pool.Close()
	readyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	err = database.Ready(readyCtx, pool)
	cancel()
	if err != nil {
		return err
	}
	logger.Info("worker started")
	stores, err := storeconfig.New(ctx, pool, os.Getenv)
	if err != nil {
		return err
	}
	store, err := objectstore.FromEnv(os.Getenv)
	if err != nil {
		return err
	}
	model, err := dialog.FromEnv(os.Getenv)
	if err != nil {
		return err
	}
	opts, err := correction.FromEnv(os.Getenv)
	if err != nil {
		return err
	}
	var corrector correction.Model
	var juror learning.StructuredDialog
	if model != nil {
		corrector = model
		juror = model
	}
	if opts.Mode == "provider" && model == nil {
		return fmt.Errorf("provider corrections require LLM configuration")
	}
	return worker.New(queue.New(pool), map[string]worker.Handler{
		"ack_store_purchase": stores.AcknowledgeHandler(),
		"system.probe":       probe.Handler(pool),
		"correct_submission": correction.Handler(pool, store, corrector, opts, logger),
		"review_oral":        learning.OralReviewHandler(pool, juror),
		"delete_object":      objectstore.DeleteHandler(store),
	}, logger).Run(ctx)
}
