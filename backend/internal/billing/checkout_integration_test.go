//go:build integration

package billing

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/pierremarsotlyon1/crpe/backend/internal/testdatabase"
)

func TestCheckoutReusesProviderStateBeforeWebhookAndAfterRollback(t *testing.T) {
	db := testdatabase.Open(t)
	ctx := t.Context()
	var user string
	if err := db.QueryRow(ctx, "INSERT INTO users(email,role) VALUES('checkout@example.test','student') RETURNING id::text").Scan(&user); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "INSERT INTO billing_customers VALUES($1,'cus_checkout')", user); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	sessions := []map[string]any{}
	keys := map[string]bool{}
	status := ""
	finishDuringExpiry := false
	created, expired := 0, 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		write := func(v any) {
			if err := json.NewEncoder(w).Encode(v); err != nil {
				t.Error(err)
			}
		}
		switch {
		case r.Method == "GET" && r.URL.Path == "/v1/checkout/sessions":
			if r.URL.Query().Get("customer") != "cus_checkout" {
				t.Error("checkout query omitted customer isolation")
			}
			items := []map[string]any{}
			for i := len(sessions) - 1; i >= 0; i-- {
				if r.URL.Query().Get("status") == "open" && sessions[i]["status"] != "open" {
					continue
				}
				items = append(items, sessions[i])
				if r.URL.Query().Get("limit") == "1" {
					break
				}
			}
			write(map[string]any{"data": items, "has_more": false})
		case r.Method == "GET" && r.URL.Path == "/v1/subscriptions":
			if r.URL.Query().Get("customer") != "cus_checkout" || r.URL.Query().Get("status") != "all" {
				t.Error("subscription query omitted customer or pending subscriptions")
			}
			items := []map[string]string{}
			if status != "" {
				items = append(items, map[string]string{"id": "sub_checkout", "customer": "cus_checkout", "status": status})
			}
			write(map[string]any{"data": items, "has_more": false})
		case r.Method == "POST" && r.URL.Path == "/v1/checkout/sessions":
			key := r.Header.Get("Idempotency-Key")
			if key == "" || keys[key] {
				t.Error("new logical checkout reused an expired session's idempotency key")
			}
			keys[key] = true
			for _, session := range sessions {
				if session["status"] == "open" {
					t.Error("created a second payable checkout session")
				}
			}
			created++
			item := map[string]any{"id": fmt.Sprintf("cs_fixture_%d", created), "url": fmt.Sprintf("https://checkout.stripe.com/fixture_%d", created), "customer": "cus_checkout", "client_reference_id": user, "mode": "subscription", "status": "open", "metadata": map[string]string{"plan_key": r.FormValue("metadata[plan_key]"), "price_id": r.FormValue("metadata[price_id]")}}
			sessions = append(sessions, item)
			write(item)
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/expire"):
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/checkout/sessions/"), "/expire")
			for _, session := range sessions {
				if session["id"] != id {
					continue
				}
				if finishDuringExpiry {
					session["status"], status = "complete", "active"
					http.Error(w, "fixture completed concurrently", 400)
					return
				}
				expired++
				session["status"] = "expired"
				write(session)
				return
			}
			http.NotFound(w, r)
		default:
			t.Errorf("unexpected checkout request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()
	s := Service{db: db, enabled: true, key: "fixture", endpoint: provider.URL, client: provider.Client(), webURL: "https://prep.example", plans: []planConfig{{Key: "base", Tier: "autonomie", PriceID: "price_base"}, {Key: "coach", Tier: "suivi", PriceID: "price_suivi"}}}
	// Model a process that obtains the provider URL but loses its local commit.
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, "SELECT id FROM users WHERE id=$1 FOR UPDATE", user); err != nil {
		t.Fatal(err)
	}
	first, err := s.checkout(ctx, tx, user, "checkout@example.test", "base")
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			got, err := s.Checkout(ctx, user, "checkout@example.test", "base")
			if err != nil || got != first {
				t.Errorf("concurrent retry=%q err=%v", got, err)
			}
		})
	}
	wg.Wait()
	mu.Lock()
	if created != 1 || expired != 0 {
		t.Errorf("retry created=%d expired=%d", created, expired)
	}
	sessions[0]["status"], status = "complete", "incomplete"
	mu.Unlock()
	if _, err = s.Checkout(ctx, user, "checkout@example.test", "coach"); !errors.Is(err, ErrActiveSubscription) {
		t.Fatalf("payment pending webhook allowed checkout: %v", err)
	}
	mu.Lock()
	status = "canceled"
	mu.Unlock()
	if _, err = s.Checkout(ctx, user, "checkout@example.test", "coach"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Checkout(ctx, user, "checkout@example.test", "base"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if created != 3 || expired != 1 {
		t.Errorf("plan replacement created=%d expired=%d", created, expired)
	}
	finishDuringExpiry = true
	mu.Unlock()
	if _, err = s.Checkout(ctx, user, "checkout@example.test", "coach"); err == nil {
		t.Fatal("failed expiry allowed a replacement checkout")
	}
	if _, err = s.Checkout(ctx, user, "checkout@example.test", "coach"); !errors.Is(err, ErrActiveSubscription) {
		t.Fatalf("completed checkout missing webhook allowed a second subscription: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if created != 3 {
		t.Fatalf("duplicate checkout created: %d", created)
	}
}
