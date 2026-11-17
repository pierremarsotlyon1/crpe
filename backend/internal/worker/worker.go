// Package worker runs handlers independently of the queue transport.
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

var ErrEmpty = errors.New("no delivery available")
var ErrLostLease = errors.New("delivery no longer owned")

// Message carries stable business input, never database connections or broker delivery tags.
type Message struct {
	ID      string
	Kind    string
	Payload json.RawMessage
	Attempt int
}

// Delivery owns transport acknowledgement and retry semantics. Handlers never see it.
type Delivery interface {
	Message() Message
	Ack(context.Context) error
	Retry(context.Context, time.Duration) error
}

// Consumer is implemented by queue adapters. Empty polling and disconnected
// brokers are distinct outcomes; Receive must respect cancellation.
type Consumer interface {
	Receive(context.Context) (Delivery, error)
}
type Handler func(context.Context, Message) error

type Runner struct {
	consumer Consumer
	handlers map[string]Handler
	logger   *slog.Logger
}

func New(consumer Consumer, handlers map[string]Handler, logger *slog.Logger) *Runner {
	registered := make(map[string]Handler, len(handlers))
	for kind, handler := range handlers {
		registered[kind] = handler
	}
	return &Runner{consumer: consumer, handlers: registered, logger: logger}
}

// Run stops receiving on cancellation and gives an already received task a
// bounded grace period. Effects must be idempotent: acknowledgement can fail.
func (r *Runner) Run(ctx context.Context) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for ctx.Err() == nil {
		receiveCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		delivery, err := r.consumer.Receive(receiveCtx)
		cancel()
		if err == nil {
			r.process(ctx, delivery)
			continue
		}
		if !errors.Is(err, ErrEmpty) && ctx.Err() == nil {
			r.logger.Error("receive job failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
	return nil
}

func (r *Runner) process(ctx context.Context, delivery Delivery) {
	message := delivery.Message()
	handler := r.handlers[message.Kind]
	jobCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
	defer cancel()
	err := fmt.Errorf("unsupported job kind %q", message.Kind)
	if handler != nil {
		err = handler(jobCtx, message)
	}
	if err == nil {
		err = delivery.Ack(jobCtx)
	}
	if err == nil {
		r.logger.Info("job completed", "job_id", message.ID, "kind", message.Kind, "attempt", message.Attempt)
		return
	}
	r.logger.Error("job failed", "job_id", message.ID, "kind", message.Kind, "error", err)
	if errors.Is(err, ErrLostLease) {
		return
	}
	retryCtx, retryCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer retryCancel()
	attempt := min(max(message.Attempt, 1), 8)
	delay := time.Duration(min(attempt*attempt, 60)) * time.Second
	if err := delivery.Retry(retryCtx, delay); err != nil && !errors.Is(err, ErrLostLease) {
		r.logger.Error("retry scheduling failed", "job_id", message.ID, "error", err)
	}
}
