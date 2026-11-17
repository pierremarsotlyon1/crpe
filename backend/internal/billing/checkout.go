package billing

import (
	"context"
	"errors"

	"crypto/sha256"
	"fmt"
	"github.com/jackc/pgx/v5"
	"net/http"
	"net/url"
	"strings"
)

type checkoutSession struct {
	ID, URL, Customer, Mode, Status string
	ClientReferenceID               string `json:"client_reference_id"`
	Metadata                        struct {
		PlanKey string `json:"plan_key"`
		PriceID string `json:"price_id"`
	}
}

// prepareCheckout runs under the caller's shared user-row lock. Provider state
// closes the gap before webhooks arrive and survives an interrupted DB commit.
func (s *Service) prepareCheckout(ctx context.Context, user, customer string, target planConfig) (reuse, key string, err error) {
	var open struct {
		Data    []checkoutSession
		HasMore bool `json:"has_more"`
	}
	query := url.Values{"customer": {customer}, "status": {"open"}, "limit": {"100"}}
	if err = s.request(ctx, http.MethodGet, "/v1/checkout/sessions?"+query.Encode(), nil, "", &open); err != nil {
		return "", "", err
	}
	if open.Data == nil || open.HasMore {
		return "", "", fmt.Errorf("checkout sessions require reconciliation")
	}
	var history struct{ Data []checkoutSession }
	query.Del("status")
	query.Set("limit", "1")
	if err = s.request(ctx, http.MethodGet, "/v1/checkout/sessions?"+query.Encode(), nil, "", &history); err != nil {
		return "", "", err
	}
	if history.Data == nil || len(history.Data) > 1 {
		return "", "", fmt.Errorf("invalid checkout history")
	}
	previous := "initial"
	if len(history.Data) == 1 {
		last := history.Data[0]
		if last.Customer != customer || !strings.HasPrefix(last.ID, "cs_") {
			return "", "", fmt.Errorf("invalid checkout customer")
		}
		previous = last.ID
	}
	var subscriptions struct {
		Data    []struct{ ID, Customer, Status string }
		HasMore bool `json:"has_more"`
	}
	query.Set("limit", "100")
	query.Set("status", "all")
	if err = s.request(ctx, http.MethodGet, "/v1/subscriptions?"+query.Encode(), nil, "", &subscriptions); err != nil {
		return "", "", err
	}
	if subscriptions.Data == nil || subscriptions.HasMore {
		return "", "", fmt.Errorf("subscriptions require reconciliation")
	}
	for _, sub := range subscriptions.Data {
		if sub.Customer != customer || !strings.HasPrefix(sub.ID, "sub_") {
			return "", "", fmt.Errorf("invalid subscription customer")
		}
		// Pending, paused and unpaid subscriptions also block a second purchase.
		if sub.Status != "canceled" && sub.Status != "incomplete_expired" {
			return "", "", ErrActiveSubscription
		}
	}
	keep := ""
	for _, session := range open.Data {
		if session.Customer != customer || !strings.HasPrefix(session.ID, "cs_") || session.Status != "open" {
			return "", "", fmt.Errorf("invalid open checkout session")
		}
		if session.Mode != "subscription" {
			continue
		}
		if session.ClientReferenceID != user {
			return "", "", ErrActiveSubscription
		}
		if keep == "" && session.Metadata.PlanKey == target.Key && session.Metadata.PriceID == target.PriceID {
			if !strings.HasPrefix(session.URL, "https://checkout.stripe.com/") {
				return "", "", fmt.Errorf("invalid checkout URL")
			}
			keep, reuse = session.ID, session.URL
			continue
		}
		var expired checkoutSession
		if err = s.request(ctx, http.MethodPost, "/v1/checkout/sessions/"+url.PathEscape(session.ID)+"/expire", url.Values{}, "expire-"+session.ID, &expired); err != nil {
			// It may have completed concurrently. Never create a replacement until
			// the provider confirms expiry; the next call checks subscriptions again.
			return "", "", err
		}
		if expired.ID != session.ID || expired.Status != "expired" {
			return "", "", fmt.Errorf("checkout expiry was not confirmed")
		}
	}
	// A provider-created session remains discoverable if our process dies. The
	// last session also distinguishes a new attempt from a deliberately expired one.
	digest := sha256.Sum256([]byte(user + "/" + target.Key + "/" + target.PriceID + "/" + previous))
	return reuse, fmt.Sprintf("checkout-%x", digest), nil
}

// CheckNativeOrder checks Stripe's current customer state while the caller holds the member lock.
// It covers sessions created before native orders and confirmations that precede their webhook.
func (s *Service) CheckNativeOrder(ctx context.Context, tx pgx.Tx, user string) error {
	var customer string
	e := tx.QueryRow(ctx, `SELECT stripe_customer_id FROM billing_customers WHERE user_id=$1`, user).Scan(&customer)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	if s == nil || s.key == "" {
		return ErrUnavailable
	}
	query := url.Values{"customer": {customer}, "status": {"open"}, "limit": {"100"}}
	var sessions struct {
		Data    []checkoutSession
		HasMore bool `json:"has_more"`
	}
	if e = s.request(ctx, http.MethodGet, "/v1/checkout/sessions?"+query.Encode(), nil, "", &sessions); e != nil {
		return e
	}
	if sessions.Data == nil || sessions.HasMore {
		return ErrUnavailable
	}
	for _, v := range sessions.Data {
		if v.Customer != customer {
			return ErrUnavailable
		}
		if v.Mode == "subscription" {
			return ErrActiveSubscription
		}
	}
	var subs struct {
		Data    []struct{ Customer, Status string }
		HasMore bool `json:"has_more"`
	}
	query.Set("status", "all")
	if e = s.request(ctx, http.MethodGet, "/v1/subscriptions?"+query.Encode(), nil, "", &subs); e != nil {
		return e
	}
	if subs.Data == nil || subs.HasMore {
		return ErrUnavailable
	}
	for _, v := range subs.Data {
		if v.Customer != customer {
			return ErrUnavailable
		}
		if v.Status != "canceled" && v.Status != "incomplete_expired" {
			return ErrActiveSubscription
		}
	}
	return nil
}
