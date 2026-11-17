// Package billing keeps payment provider details out of learning and client code.
package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrDisabled = errors.New("sales disabled")
var ErrUnavailable = errors.New("payment provider not configured")
var ErrInvalidPlan = errors.New("unknown billing plan")
var ErrActiveSubscription = errors.New("active subscription")
var ErrRefundConflict = errors.New("refund requires reconciliation")

type Plan struct {
	Tier     string `json:"tier"`
	Key      string `json:"key"`
	Name     string `json:"name"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
	Interval string `json:"interval"`
}
type planConfig struct {
	Tier    string `json:"tier"`
	Key     string `json:"key"`
	Name    string `json:"name"`
	PriceID string `json:"priceId"`
}
type Invoice struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
	URL      string `json:"url"`
	Created  int64  `json:"created"`
}
type Service struct {
	db                                   *pgxpool.Pool
	client                               *http.Client
	enabled                              bool
	key, webhookSecret, webURL, endpoint string
	plans                                []planConfig
}

func New(db *pgxpool.Pool, lookup func(string) string) (*Service, error) {
	enabled := false
	if value := lookup("SALES_ENABLED"); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return nil, fmt.Errorf("invalid SALES_ENABLED")
		}
		enabled = parsed
	}
	s := &Service{db: db, client: &http.Client{Timeout: 20 * time.Second}, enabled: enabled, key: lookup("STRIPE_SECRET_KEY"), webhookSecret: lookup("STRIPE_WEBHOOK_SECRET"), webURL: strings.TrimRight(lookup("PUBLIC_WEB_URL"), "/"), endpoint: "https://api.stripe.com"}
	if s.webURL == "" {
		s.webURL = "http://localhost:3000"
	}
	if value := lookup("BILLING_PLANS_JSON"); value != "" {
		if err := json.Unmarshal([]byte(value), &s.plans); err != nil {
			return nil, fmt.Errorf("invalid BILLING_PLANS_JSON")
		}
	}
	seen := map[string]bool{}
	prices := map[string]bool{}
	for i := range s.plans {
		p := &s.plans[i]
		if p.Tier == "" {
			p.Tier = "autonomie"
		}
		if p.Tier != "autonomie" && p.Tier != "suivi" {
			return nil, fmt.Errorf("unknown billing plan tier")
		}
		if p.Key == "" || p.Name == "" || !strings.HasPrefix(p.PriceID, "price_") || seen[p.Key] || prices[p.PriceID] {
			return nil, fmt.Errorf("billing plans need unique keys, names and Stripe price IDs")
		}
		seen[p.Key] = true
		prices[p.PriceID] = true
	}
	return s, nil
}
func (s *Service) management() error {
	if s.key == "" {
		return ErrUnavailable
	}
	return nil
}
func (s *Service) available() error {
	if !s.enabled {
		return ErrDisabled
	}
	if s.key == "" || len(s.plans) == 0 {
		return ErrUnavailable
	}
	return nil
}

func (s *Service) Plans(ctx context.Context) ([]Plan, error) {
	if err := s.available(); err != nil {
		return nil, err
	}
	plans := make([]Plan, 0, len(s.plans))
	for _, p := range s.plans {
		var price struct {
			Active     bool   `json:"active"`
			UnitAmount int64  `json:"unit_amount"`
			Currency   string `json:"currency"`
			Recurring  *struct {
				Interval      string `json:"interval"`
				IntervalCount int    `json:"interval_count"`
			} `json:"recurring"`
		}
		if err := s.request(ctx, http.MethodGet, "/v1/prices/"+url.PathEscape(p.PriceID), nil, "", &price); err != nil {
			return nil, err
		}
		if !price.Active || price.Recurring == nil || price.Recurring.IntervalCount != 1 || (price.Recurring.Interval != "month" && price.Recurring.Interval != "year") {
			return nil, fmt.Errorf("configured price must be an active monthly or annual subscription")
		}
		plans = append(plans, Plan{Tier: p.Tier, Key: p.Key, Name: p.Name, Amount: price.UnitAmount, Currency: price.Currency, Interval: price.Recurring.Interval})
	}
	return plans, nil
}

func (s *Service) Checkout(ctx context.Context, userID, email, planKey string) (string, error) {
	if err := s.available(); err != nil {
		return "", err
	}
	var checkoutURL string
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var exists string
		if err := tx.QueryRow(ctx, "SELECT id::text FROM users WHERE id=$1 FOR UPDATE", userID).Scan(&exists); err != nil {
			return err
		}
		var blocked bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT FROM store_memberships WHERE user_id=$1 AND NOT superseded AND (state='pending' OR state='active' AND expires_at>now())) OR EXISTS(SELECT FROM store_orders WHERE user_id=$1 AND platform<>'stripe' AND expires_at>now())`, userID).Scan(&blocked); err != nil {
			return err
		}
		if blocked {
			return ErrActiveSubscription
		}
		var err error
		checkoutURL, err = s.checkout(ctx, tx, userID, email, planKey)
		if err == nil && strings.HasPrefix(checkoutURL, "https://checkout.stripe.com/") {
			_, err = tx.Exec(ctx, `INSERT INTO store_orders(user_id,platform,expires_at) VALUES($1,'stripe',now()+interval '24 hours') ON CONFLICT(user_id) DO UPDATE SET platform='stripe',expires_at=excluded.expires_at`, userID)
		}
		return err
	})
	return checkoutURL, err
}

func (s *Service) checkout(ctx context.Context, tx pgx.Tx, userID, email, planKey string) (string, error) {
	if err := s.available(); err != nil {
		return "", err
	}
	var selected planConfig
	for _, p := range s.plans {
		if p.Key == planKey {
			selected = p
			break
		}
	}
	if selected.PriceID == "" {
		return "", ErrInvalidPlan
	}
	if target, active, err := s.prepareUpgrade(ctx, tx, userID, selected); active || err != nil {
		return target, err
	}
	customer, err := s.customer(ctx, tx, userID, email)
	if err != nil {
		return "", err
	}
	reuse, key, err := s.prepareCheckout(ctx, userID, customer, selected)
	if err != nil || reuse != "" {
		return reuse, err
	}
	form := url.Values{"mode": {"subscription"}, "customer": {customer}, "client_reference_id": {userID}, "line_items[0][price]": {selected.PriceID}, "line_items[0][quantity]": {"1"}, "success_url": {s.webURL + "/espace/abonnement?retour=succes"}, "cancel_url": {s.webURL + "/offres"}, "subscription_data[metadata][user_id]": {userID}, "subscription_data[metadata][plan_key]": {planKey}, "metadata[plan_key]": {planKey}, "metadata[price_id]": {selected.PriceID}, "locale": {"fr"}}
	var result struct {
		URL string `json:"url"`
	}
	if err := s.request(ctx, http.MethodPost, "/v1/checkout/sessions", form, key, &result); err != nil {
		return "", err
	}
	if !strings.HasPrefix(result.URL, "https://checkout.stripe.com/") {
		return "", fmt.Errorf("invalid checkout URL")
	}
	return result.URL, nil
}

func (s *Service) customer(ctx context.Context, tx pgx.Tx, userID, email string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `SELECT stripe_customer_id FROM billing_customers WHERE user_id=$1`, userID).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := s.request(ctx, http.MethodPost, "/v1/customers", url.Values{"email": {email}, "metadata[user_id]": {userID}}, "customer-"+userID, &result); err != nil {
		return "", err
	}
	if !strings.HasPrefix(result.ID, "cus_") {
		return "", fmt.Errorf("invalid customer response")
	}
	_, err = tx.Exec(ctx, `INSERT INTO billing_customers(user_id,stripe_customer_id) VALUES($1,$2) ON CONFLICT(user_id) DO NOTHING`, userID, result.ID)
	if err != nil {
		return "", err
	}
	err = tx.QueryRow(ctx, `SELECT stripe_customer_id FROM billing_customers WHERE user_id=$1`, userID).Scan(&id)
	return id, err
}

func (s *Service) Portal(ctx context.Context, userID string) (string, error) {
	if err := s.management(); err != nil {
		return "", err
	}
	var customer string
	if err := s.db.QueryRow(ctx, `SELECT stripe_customer_id FROM billing_customers WHERE user_id=$1`, userID).Scan(&customer); err != nil {
		return "", err
	}
	var result struct {
		URL string `json:"url"`
	}
	if err := s.request(ctx, http.MethodPost, "/v1/billing_portal/sessions", url.Values{"customer": {customer}, "return_url": {s.webURL + "/espace/abonnement"}, "locale": {"fr"}}, "", &result); err != nil {
		return "", err
	}
	if !strings.HasPrefix(result.URL, "https://billing.stripe.com/") {
		return "", fmt.Errorf("invalid billing portal URL")
	}
	return result.URL, nil
}

func (s *Service) Access(ctx context.Context, userID string) (bool, error) {
	if !s.enabled {
		return true, nil
	}
	var active bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM member_rights WHERE user_id=$1 AND valid_until>now() AND NOT refunded)`, userID).Scan(&active)
	return active, err
}

func (s *Service) Invoices(ctx context.Context, userID string) ([]Invoice, error) {
	var customer string
	err := s.db.QueryRow(ctx, `SELECT stripe_customer_id FROM billing_customers WHERE user_id=$1`, userID).Scan(&customer)
	if errors.Is(err, pgx.ErrNoRows) {
		return []Invoice{}, nil
	}
	if err != nil {
		return nil, err
	}
	if err := s.management(); err != nil {
		return nil, err
	}
	var result struct {
		Data []struct {
			ID         string `json:"id"`
			Status     string `json:"status"`
			AmountPaid int64  `json:"amount_paid"`
			Currency   string `json:"currency"`
			URL        string `json:"hosted_invoice_url"`
			Created    int64  `json:"created"`
		} `json:"data"`
	}
	if err := s.request(ctx, http.MethodGet, "/v1/invoices?customer="+url.QueryEscape(customer)+"&limit=100", nil, "", &result); err != nil {
		return nil, err
	}
	items := make([]Invoice, 0, len(result.Data))
	for _, v := range result.Data {
		items = append(items, Invoice{ID: v.ID, Status: v.Status, Amount: v.AmountPaid, Currency: v.Currency, URL: v.URL, Created: v.Created})
	}
	return items, nil
}

func (s *Service) request(ctx context.Context, method, path string, form url.Values, idempotency string, out any) error {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, s.endpoint+path, body)
	if err != nil {
		return err
	}
	req.SetBasicAuth(s.key, "")
	req.Header.Set("Stripe-Version", "2026-10-28.endive")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if idempotency != "" {
		req.Header.Set("Idempotency-Key", idempotency)
	}
	res, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("payment provider request failed")
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("payment provider returned HTTP %d", res.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(out); err != nil {
		return fmt.Errorf("invalid payment provider response")
	}
	return nil
}

// SupportPlans returns configured plan keys that grant teacher-led support.
func (s *Service) SupportPlans() []string {
	keys := []string{}
	if s != nil {
		for _, p := range s.plans {
			if p.Tier == "suivi" {
				keys = append(keys, p.Key)
			}
		}
	}
	return keys
}
