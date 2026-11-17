package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pierremarsotlyon1/crpe/backend/internal/config"
	"github.com/pierremarsotlyon1/crpe/backend/internal/database"
	"github.com/pierremarsotlyon1/crpe/backend/internal/queue"
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
	return queue.New(pool).Run(ctx, logger)
}
