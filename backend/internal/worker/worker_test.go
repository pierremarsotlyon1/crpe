package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

type testDelivery struct {
	message Message
	ack     func(context.Context) error
	retry   func(context.Context, time.Duration) error
}

func (d testDelivery) Message() Message              { return d.message }
func (d testDelivery) Ack(ctx context.Context) error { return d.ack(ctx) }
func (d testDelivery) Retry(ctx context.Context, delay time.Duration) error {
	return d.retry(ctx, delay)
}

type testConsumer struct {
	receive func(context.Context) (Delivery, error)
}

func (c testConsumer) Receive(ctx context.Context) (Delivery, error) { return c.receive(ctx) }

func TestHandlerRunsWithoutTransportAndDrainsOnShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	acknowledged := false
	receives := 0
	consumer := testConsumer{receive: func(context.Context) (Delivery, error) {
		receives++
		return testDelivery{
			message: Message{ID: "portable-id", Kind: "correction", Attempt: 1},
			ack: func(ctx context.Context) error {
				if ctx.Err() != nil {
					t.Fatal("ack cancelled during grace")
				}
				acknowledged = true
				return nil
			},
			retry: func(context.Context, time.Duration) error { t.Fatal("successful handler retried"); return nil },
		}, nil
	}}
	handler := func(ctx context.Context, message Message) error {
		if message.ID != "portable-id" {
			t.Fatal("message lost")
		}
		cancel()
		if ctx.Err() != nil {
			t.Fatal("in-flight task cancelled instead of drained")
		}
		return nil
	}
	runner := New(consumer, map[string]Handler{"correction": handler}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := runner.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if receives != 1 || !acknowledged {
		t.Fatalf("receives=%d ack=%v", receives, acknowledged)
	}
}

func TestFailedAcknowledgementRetriesAnIdempotentHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	effect := 0
	retried := false
	consumer := testConsumer{receive: func(context.Context) (Delivery, error) {
		return testDelivery{
			message: Message{ID: "stable-business-id", Kind: "correction", Attempt: 2},
			ack:     func(context.Context) error { return errors.New("connection lost") },
			retry: func(ctx context.Context, delay time.Duration) error {
				if delay != 4*time.Second || ctx.Err() != nil {
					t.Fatal("invalid retry")
				}
				retried = true
				cancel()
				return nil
			},
		}, nil
	}}
	runner := New(consumer, map[string]Handler{"correction": func(context.Context, Message) error { effect++; return nil }}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := runner.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if effect != 1 || !retried {
		t.Fatalf("effect=%d retry=%v", effect, retried)
	}
}

func TestLostOwnershipDoesNotRescheduleAnotherConsumersMessage(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	consumer := testConsumer{receive: func(context.Context) (Delivery, error) {
		return testDelivery{
			message: Message{Kind: "correction"},
			ack:     func(context.Context) error { cancel(); return ErrLostLease },
			retry:   func(context.Context, time.Duration) error { t.Fatal("lost delivery was retried"); return nil },
		}, nil
	}}
	runner := New(consumer, map[string]Handler{"correction": func(context.Context, Message) error { return nil }}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := runner.Run(ctx); err != nil {
		t.Fatal(err)
	}
}
