package billing

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
)

type stripeRefund struct {
	ID     string
	Status string
}

func (s *Service) Refund(ctx context.Context, actor, intent string) (string, error) {
	if err := s.management(); err != nil {
		return "", err
	}
	if !strings.HasPrefix(intent, "pi_") {
		return "", fmt.Errorf("invalid payment intent")
	}
	var refundID string
	var existing bool
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		// Store the provider result in its own transaction. Cancellation of the
		// original subscription can then be safely retried after a provider outage.
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,1101))", intent); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, "SELECT stripe_refund_id FROM billing_refunds WHERE payment_intent_id=$1", intent).Scan(&refundID)
		if err == nil {
			existing = true
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var customer, sub string
		var refunded bool
		if err = tx.QueryRow(ctx, `SELECT coalesce(c.stripe_customer_id,''),p.stripe_subscription_id,p.refunded FROM billing_payments p LEFT JOIN billing_customers c ON c.user_id=p.user_id WHERE p.payment_intent_id=$1`, intent).Scan(&customer, &sub, &refunded); err != nil {
			return err
		}
		if refunded {
			return ErrRefundConflict
		}
		if sub == "" && customer != "" {
			sub, err = s.paymentSubscription(ctx, intent, customer)
			if err != nil {
				return err
			}
		}
		if _, err = tx.Exec(ctx, "UPDATE billing_payments SET stripe_subscription_id=$2 WHERE payment_intent_id=$1", intent, sub); err != nil {
			return err
		}
		var result stripeRefund
		if err = s.request(ctx, http.MethodPost, "/v1/refunds", url.Values{"payment_intent": {intent}}, "full-refund-"+intent, &result); err != nil {
			return err
		}
		if !strings.HasPrefix(result.ID, "re_") || (result.Status != "succeeded" && result.Status != "pending" && result.Status != "requires_action") {
			return fmt.Errorf("refund was not accepted")
		}
		refundID = result.ID
		_, err = tx.Exec(ctx, "INSERT INTO billing_refunds(payment_intent_id,actor_id,stripe_refund_id,provider_status) VALUES($1,$2,$3,$4)", intent, actor, refundID, result.Status)
		return err
	})
	if err != nil {
		return "", err
	}
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error { return s.settleRefund(ctx, tx, intent, existing) })
	if err != nil {
		return "", err
	}
	var current string
	if err = s.db.QueryRow(ctx, "SELECT provider_status FROM billing_refunds WHERE payment_intent_id=$1", intent).Scan(&current); err != nil {
		return "", err
	}
	if current == "failed" || current == "canceled" {
		return "", ErrRefundConflict
	}
	return refundID, nil
}
func (s *Service) settleRefund(ctx context.Context, tx pgx.Tx, intent string, refresh bool) error {
	var id, status, sub string
	var finished bool
	err := tx.QueryRow(ctx, `SELECT r.stripe_refund_id,r.provider_status,p.stripe_subscription_id,r.finalized_at IS NOT NULL FROM billing_refunds r JOIN billing_payments p USING(payment_intent_id) WHERE r.payment_intent_id=$1 FOR UPDATE OF r`, intent).Scan(&id, &status, &sub, &finished)
	if err != nil || finished {
		return err
	}
	if refresh && status != "succeeded" {
		var current stripeRefund
		if err = s.request(ctx, http.MethodGet, "/v1/refunds/"+url.PathEscape(id), nil, "", &current); err != nil {
			return err
		}
		if current.ID != id {
			return fmt.Errorf("provider returned mismatched refund")
		}
		status = current.Status
		if _, err = tx.Exec(ctx, "UPDATE billing_refunds SET provider_status=$2 WHERE payment_intent_id=$1", intent, status); err != nil {
			return err
		}
	}
	if status != "succeeded" {
		return nil
	}
	if sub != "" {
		if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", sub); err != nil {
			return err
		}
		var current struct {
			ID     string
			Status string
		}
		if err = s.request(ctx, http.MethodGet, "/v1/subscriptions/"+url.PathEscape(sub), nil, "", &current); err != nil {
			return err
		}
		if current.ID != sub {
			return fmt.Errorf("provider returned mismatched subscription")
		}
		if current.Status != "canceled" {
			var cancelled struct {
				ID     string
				Status string
			}
			if err = s.request(ctx, http.MethodDelete, "/v1/subscriptions/"+url.PathEscape(sub), nil, "cancel-refund-"+intent, &cancelled); err != nil {
				return err
			}
			if cancelled.ID != sub || cancelled.Status != "canceled" {
				return fmt.Errorf("subscription cancellation unconfirmed")
			}
		}
		if _, err = tx.Exec(ctx, "UPDATE entitlements SET refunded=true,valid_until=least(valid_until,now()) WHERE stripe_subscription_id=$1", sub); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, "UPDATE billing_payments SET refunded=true WHERE payment_intent_id=$1", intent); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "UPDATE billing_refunds SET finalized_at=now() WHERE payment_intent_id=$1", intent)
	return err
}
