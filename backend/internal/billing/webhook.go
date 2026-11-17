package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrSignature = fmt.Errorf("invalid webhook signature")

func verifySignature(body []byte, signature, secret string, now time.Time) error {
	if secret == "" || len(body) > 1<<20 {
		return ErrSignature
	}
	var timestamp string
	signatures := []string{}
	for _, field := range strings.Split(signature, ",") {
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		if key == "t" {
			timestamp = value
		}
		if key == "v1" {
			signatures = append(signatures, value)
		}
	}
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return ErrSignature
	}
	if delta := now.Sub(time.Unix(ts, 0)); delta > 5*time.Minute || delta < -5*time.Minute {
		return ErrSignature
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "."))
	_, _ = mac.Write(body)
	expected := mac.Sum(nil)
	for _, value := range signatures {
		actual, err := hex.DecodeString(value)
		if err == nil && hmac.Equal(expected, actual) {
			return nil
		}
	}
	return ErrSignature
}

// Webhook verifies raw bytes and atomically records the event with its effect.
// For subscription events the current provider state is fetched, so delayed
// events cannot restore an obsolete subscription status.
func (s *Service) Webhook(ctx context.Context, body []byte, signature string) error {
	if s.key == "" || s.webhookSecret == "" {
		return ErrUnavailable
	}
	if err := verifySignature(body, signature, s.webhookSecret, time.Now()); err != nil {
		return err
	}
	var event struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Data struct {
			Object json.RawMessage `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &event); err != nil || event.ID == "" {
		return fmt.Errorf("invalid billing event")
	}
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		insert, err := tx.Exec(ctx, `INSERT INTO billing_events(id,kind) VALUES($1,$2) ON CONFLICT DO NOTHING`, event.ID, event.Type)
		if err != nil || insert.RowsAffected() == 0 {
			return err
		}
		switch event.Type {
		case "checkout.session.completed", "checkout.session.async_payment_succeeded":
			var v struct {
				Subscription  string `json:"subscription"`
				PaymentStatus string `json:"payment_status"`
			}
			if err := json.Unmarshal(event.Data.Object, &v); err != nil {
				return err
			}
			if v.Subscription == "" || v.PaymentStatus != "paid" {
				return nil
			}
			return s.syncSubscription(ctx, tx, v.Subscription)
		case "customer.subscription.updated", "customer.subscription.deleted", "customer.subscription.created":
			var v struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(event.Data.Object, &v); err != nil {
				return err
			}
			return s.syncSubscription(ctx, tx, v.ID)
		case "refund.created", "refund.updated", "refund.failed":
			var v struct{ ID string }
			if err := json.Unmarshal(event.Data.Object, &v); err != nil {
				return err
			}
			if !strings.HasPrefix(v.ID, "re_") {
				return fmt.Errorf("invalid refund event")
			}
			var intent string
			err := tx.QueryRow(ctx, "SELECT payment_intent_id FROM billing_refunds WHERE stripe_refund_id=$1", v.ID).Scan(&intent)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			if err != nil {
				return err
			}
			return s.settleRefund(ctx, tx, intent, true)
		case "invoice.paid":
			var v struct {
				ID            string `json:"id"`
				Customer      string `json:"customer"`
				Amount        int64  `json:"amount_paid"`
				Currency      string `json:"currency"`
				PaymentIntent string `json:"payment_intent"`
				Subscription  string `json:"subscription"`
				Parent        struct {
					SubscriptionDetails struct {
						Subscription string `json:"subscription"`
					} `json:"subscription_details"`
				} `json:"parent"`
			}
			if err := json.Unmarshal(event.Data.Object, &v); err != nil {
				return err
			}
			if v.Subscription == "" {
				v.Subscription = v.Parent.SubscriptionDetails.Subscription
			}
			return s.invoicePaid(ctx, tx, v.ID, v.Customer, v.Subscription, v.PaymentIntent, v.Currency, v.Amount)
		case "charge.succeeded", "charge.refunded":
			var v struct {
				Customer      string `json:"customer"`
				Amount        int64  `json:"amount"`
				Currency      string `json:"currency"`
				PaymentIntent string `json:"payment_intent"`
				Refunded      bool   `json:"refunded"`
			}
			if err := json.Unmarshal(event.Data.Object, &v); err != nil {
				return err
			}
			return s.recordPayment(ctx, tx, v.PaymentIntent, v.Customer, "", v.Amount, v.Currency, v.Refunded)
		}
		return nil
	})
}

func (s *Service) syncSubscription(ctx context.Context, tx pgx.Tx, id string) error {
	if !strings.HasPrefix(id, "sub_") {
		return fmt.Errorf("invalid subscription")
	}
	// Serialize before re-fetching provider state, preventing two concurrently
	// delivered events from committing snapshots in the opposite order.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, id); err != nil {
		return err
	}
	var v struct {
		ID               string            `json:"id"`
		Customer         string            `json:"customer"`
		Status           string            `json:"status"`
		Metadata         map[string]string `json:"metadata"`
		CurrentPeriodEnd int64             `json:"current_period_end"`
		Items            struct {
			Data []struct {
				CurrentPeriodEnd int64 `json:"current_period_end"`
				Price            struct {
					ID string `json:"id"`
				} `json:"price"`
			} `json:"data"`
		} `json:"items"`
	}
	if err := s.request(ctx, http.MethodGet, "/v1/subscriptions/"+url.PathEscape(id), nil, "", &v); err != nil {
		return err
	}
	var userID string
	if err := tx.QueryRow(ctx, `SELECT user_id::text FROM billing_customers WHERE stripe_customer_id=$1`, v.Customer).Scan(&userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	planKey := ""
	for _, item := range v.Items.Data {
		for _, plan := range s.plans {
			if item.Price.ID == plan.PriceID {
				planKey = plan.Key
				if item.CurrentPeriodEnd > v.CurrentPeriodEnd {
					v.CurrentPeriodEnd = item.CurrentPeriodEnd
				}
			}
		}
	}
	if planKey == "" {
		return fmt.Errorf("subscription price not in configured plans")
	}
	expires := time.Unix(v.CurrentPeriodEnd, 0)
	if v.Status != "active" && v.Status != "trialing" {
		expires = time.Unix(0, 0)
	}
	if v.ID != id {
		return fmt.Errorf("provider returned mismatched subscription")
	}
	_, err := tx.Exec(ctx, `INSERT INTO entitlements(user_id,plan_key,valid_until,source,stripe_customer_id,stripe_subscription_id,refunded)
 VALUES($1,$2,$3,'stripe',$4,$5,EXISTS(SELECT FROM billing_payments WHERE stripe_subscription_id=$5 AND refunded))
 ON CONFLICT(user_id,source,stripe_subscription_id) DO UPDATE SET plan_key=excluded.plan_key,
 valid_until=excluded.valid_until,refunded=entitlements.refunded OR excluded.refunded,
 stripe_customer_id=excluded.stripe_customer_id`, userID, planKey, expires, v.Customer, v.ID)
	return err
}
