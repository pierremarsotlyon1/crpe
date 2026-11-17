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

type invoicePayment struct {
	ID       string `json:"id"`
	Amount   int64  `json:"amount_paid"`
	Currency string `json:"currency"`
	Payment  struct {
		Type   string `json:"type"`
		Intent string `json:"payment_intent"`
	} `json:"payment"`
	Invoice struct {
		Customer string `json:"customer"`
		Parent   struct {
			SubscriptionDetails struct {
				ID string `json:"subscription"`
			} `json:"subscription_details"`
		} `json:"parent"`
	} `json:"invoice"`
}

func (s *Service) paymentSubscription(ctx context.Context, intent, customer string) (string, error) {
	q := url.Values{"payment[type]": {"payment_intent"}, "payment[payment_intent]": {intent}, "expand[]": {"data.invoice"}, "limit": {"100"}}
	var result struct {
		Data []invoicePayment
		More bool `json:"has_more"`
	}
	if err := s.request(ctx, http.MethodGet, "/v1/invoice_payments?"+q.Encode(), nil, "", &result); err != nil {
		return "", err
	}
	if result.More {
		return "", fmt.Errorf("payment spans too many invoices")
	}
	var id string
	for _, p := range result.Data {
		sub := p.Invoice.Parent.SubscriptionDetails.ID
		if p.Invoice.Customer != customer || sub != "" && id != "" && sub != id {
			return "", fmt.Errorf("ambiguous subscription for payment")
		}
		if sub != "" {
			id = sub
		}
	}
	return id, nil
}
func (s *Service) recordPayment(ctx context.Context, tx pgx.Tx, intent, customer, subscription string, amount int64, currency string, refunded bool) error {
	if !strings.HasPrefix(intent, "pi_") || customer == "" {
		return nil
	}
	var user string
	err := tx.QueryRow(ctx, "SELECT user_id::text FROM billing_customers WHERE stripe_customer_id=$1", customer).Scan(&user)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if refunded {
		// A charge notification can arrive before a refund leaves pending status.
		// Use the status of any locally requested refund before revoking access.
		var local bool
		if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT FROM billing_refunds WHERE payment_intent_id=$1)", intent).Scan(&local); err != nil {
			return err
		}
		if local {
			if err = s.settleRefund(ctx, tx, intent, true); err != nil {
				return err
			}
			if err = tx.QueryRow(ctx, "SELECT finalized_at IS NOT NULL FROM billing_refunds WHERE payment_intent_id=$1", intent).Scan(&refunded); err != nil {
				return err
			}
		}
	}
	if subscription == "" && refunded {
		subscription, err = s.paymentSubscription(ctx, intent, customer)
		if err != nil {
			return err
		}
	}
	// Take the subscription lock before writing the payment. Later syncs see a
	// durable full-refund marker even if this subscription was never seen before.
	if subscription != "" {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", subscription); err != nil {
			return err
		}
	}
	var effective string
	var revoked bool
	err = tx.QueryRow(ctx, `INSERT INTO billing_payments(payment_intent_id,user_id,amount,currency,refunded,stripe_subscription_id)
 VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(payment_intent_id) DO UPDATE SET
 refunded=billing_payments.refunded OR excluded.refunded,
 stripe_subscription_id=coalesce(nullif(billing_payments.stripe_subscription_id,''),excluded.stripe_subscription_id)
 RETURNING stripe_subscription_id,refunded`, intent, user, amount, currency, refunded, subscription).Scan(&effective, &revoked)
	if err != nil {
		return err
	}
	if revoked && effective != "" {
		_, err = tx.Exec(ctx, `UPDATE entitlements SET refunded=true,valid_until=least(valid_until,now()) WHERE stripe_subscription_id=$1 AND user_id=$2`, effective, user)
	}
	return err
}
func (s *Service) invoicePaid(ctx context.Context, tx pgx.Tx, invoiceID, customer, subscription, intent, currency string, amount int64) error {
	if intent != "" {
		if err := s.recordPayment(ctx, tx, intent, customer, subscription, amount, currency, false); err != nil {
			return err
		}
	} else if invoiceID != "" {
		var known bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT FROM billing_customers WHERE stripe_customer_id=$1)", customer).Scan(&known); err != nil || !known {
			return err
		}
		q := url.Values{"invoice": {invoiceID}, "status": {"paid"}, "limit": {"100"}}
		for {
			var page struct {
				Data []struct {
					ID       string
					Amount   int64 `json:"amount_paid"`
					Currency string
					Payment  struct {
						Intent string `json:"payment_intent"`
					}
				}
				More bool `json:"has_more"`
			}
			if err := s.request(ctx, http.MethodGet, "/v1/invoice_payments?"+q.Encode(), nil, "", &page); err != nil {
				return err
			}
			for _, p := range page.Data {
				if p.Payment.Intent != "" {
					if err := s.recordPayment(ctx, tx, p.Payment.Intent, customer, subscription, p.Amount, p.Currency, false); err != nil {
						return err
					}
				}
			}
			if !page.More {
				break
			}
			if len(page.Data) == 0 {
				return errors.New("invalid invoice payment pagination")
			}
			next := page.Data[len(page.Data)-1].ID
			if next == "" || next == q.Get("starting_after") {
				return errors.New("invalid invoice payment cursor")
			}
			q.Set("starting_after", next)
		}
	}
	if subscription != "" {
		return s.syncSubscription(ctx, tx, subscription)
	}
	return nil
}
