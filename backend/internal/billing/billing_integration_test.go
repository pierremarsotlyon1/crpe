//go:build integration

package billing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pierremarsotlyon1/crpe/backend/internal/testdatabase"
)

func TestWebhookOrderingAndRefundRevocation(t *testing.T) {
	db := testdatabase.Open(t)
	ctx := t.Context()
	var uid string
	if err := db.QueryRow(ctx, "INSERT INTO users(email,role) VALUES('member@example.fr','student') RETURNING id::text").Scan(&uid); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "INSERT INTO billing_customers VALUES($1,'cus_test')", uid); err != nil {
		t.Fatal(err)
	}
	var active atomic.Bool
	active.Store(true)
	var calls, refunds atomic.Int32
	var pending atomic.Bool
	pending.Store(true)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/v1/customers/cus_test":
			if _, err := fmt.Fprint(w, `{"id":"cus_test"}`); err != nil {
				t.Error(err)
			}
		case r.Method == "GET" && r.URL.Path == "/v1/invoice_payments":
			sub := "sub_test"
			if r.URL.Query().Get("payment[payment_intent]") == "pi_async" {
				sub = "sub_later"
			}
			if r.URL.Query().Get("payment[payment_intent]") == "pi_early" {
				sub = "sub_early"
			}
			if _, err := fmt.Fprintf(w, `{"has_more":false,"data":[{"id":"inpay_test","payment":{"type":"payment_intent","payment_intent":"pi_test"},"invoice":{"customer":"cus_test","parent":{"subscription_details":{"subscription":%q}}}}]}`, sub); err != nil {
				t.Error(err)
			}
		case r.Method == "GET" && (r.URL.Path == "/v1/subscriptions/sub_early" || r.URL.Path == "/v1/subscriptions/sub_later"):
			if _, err := fmt.Fprintf(w, `{"id":%q,"status":"active","customer":"cus_test","items":{"data":[{"price":{"id":"price_test"},"current_period_end":%d}]}}`, strings.TrimPrefix(r.URL.Path, "/v1/subscriptions/"), time.Now().Add(time.Hour).Unix()); err != nil {
				t.Error(err)
			}
		case r.Method == "GET" && r.URL.Path == "/v1/subscriptions/sub_test":
			calls.Add(1)
			state := "active"
			if !active.Load() {
				state = "canceled"
			}
			if _, err := fmt.Fprintf(w, `{"id":"sub_test","status":%q,"customer":"cus_test","items":{"data":[{"price":{"id":"price_test"},"current_period_end":%d}]}}`, state, time.Now().Add(time.Hour).Unix()); err != nil {
				t.Error(err)
			}
		case r.Method == "GET" && r.URL.Path == "/v1/refunds/re_async":
			status := "pending"
			if !pending.Load() {
				status = "succeeded"
			}
			if _, err := fmt.Fprintf(w, `{"id":"re_async","status":%q}`, status); err != nil {
				t.Error(err)
			}
		case r.Method == "DELETE" && r.URL.Path == "/v1/subscriptions/sub_later":
			if _, err := fmt.Fprint(w, `{"id":"sub_later","status":"canceled"}`); err != nil {
				t.Error(err)
			}
		case r.Method == "DELETE" && r.URL.Path == "/v1/subscriptions/sub_test":
			if _, err := fmt.Fprint(w, `{"id":"sub_test","status":"canceled"}`); err != nil {
				t.Error(err)
			}
		case r.Method == "POST" && r.URL.Path == "/v1/refunds":
			if r.FormValue("payment_intent") == "pi_async" {
				if _, err := fmt.Fprint(w, `{"id":"re_async","status":"pending"}`); err != nil {
					t.Error(err)
				}
				return
			}
			refunds.Add(1)
			if _, err := fmt.Fprint(w, `{"id":"re_test","status":"succeeded"}`); err != nil {
				t.Error(err)
			}
		case r.Method == "DELETE" && r.URL.Path == "/v1/customers/cus_test":
			http.Error(w, "fixture unavailable", http.StatusServiceUnavailable)
		default:
			t.Errorf("unexpected provider request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()
	s := &Service{db: db, client: provider.Client(), endpoint: provider.URL, key: "sk_test_fixture", webhookSecret: "whsec_test_fixture", enabled: true, plans: []planConfig{{Key: "autonomie", PriceID: "price_test"}}}
	send := func(id, kind string, obj any) {
		t.Helper()
		body, err := json.Marshal(map[string]any{"id": id, "type": kind, "data": map[string]any{"object": obj}})
		if err != nil {
			t.Fatal(err)
		}
		stamp := fmt.Sprintf("%d", time.Now().Unix())
		mac := hmac.New(sha256.New, []byte(s.webhookSecret))
		mac.Write([]byte(stamp + "."))
		mac.Write(body)
		sig := "t=" + stamp + ",v1=" + hex.EncodeToString(mac.Sum(nil))
		if err = s.Webhook(ctx, body, sig); err != nil {
			t.Fatal(err)
		}
	}
	access := func(want bool) {
		t.Helper()
		ok, err := s.Access(ctx, uid)
		if err != nil || ok != want {
			t.Fatalf("access=%v want=%v err=%v", ok, want, err)
		}
	}
	sub := map[string]any{"id": "sub_test"}
	send("evt_early_refund", "charge.refunded", map[string]any{"customer": "cus_test", "payment_intent": "pi_early", "amount": 1500, "currency": "eur", "refunded": true})
	send("evt_early_sub", "customer.subscription.created", map[string]any{"id": "sub_early"})
	access(false)
	send("evt_start", "customer.subscription.created", sub)
	access(true)
	send("evt_start", "customer.subscription.created", sub)
	if calls.Load() != 1 {
		t.Fatalf("duplicate event fetched provider %d times", calls.Load())
	}
	active.Store(false)
	send("evt_end", "customer.subscription.deleted", sub)
	access(false)
	send("evt_delayed", "customer.subscription.created", sub)
	access(false)
	active.Store(true)
	send("evt_new", "customer.subscription.updated", sub)
	access(true)
	charge := map[string]any{"customer": "cus_test", "payment_intent": "pi_test", "amount": 1500, "currency": "eur", "refunded": false}
	send("evt_charge", "charge.succeeded", charge)
	if id, err := s.Refund(ctx, uid, "pi_test"); err != nil || id != "re_test" {
		t.Fatal(id, err)
	}
	access(false)
	charge["refunded"] = true
	send("evt_refund", "charge.refunded", charge)
	send("evt_after_refund", "customer.subscription.updated", sub)
	access(false)
	if _, err := s.Refund(ctx, uid, "pi_test"); err != nil || refunds.Load() != 1 {
		t.Fatalf("replayed refund calls=%d err=%v", refunds.Load(), err)
	}
	send("evt_later", "customer.subscription.created", map[string]any{"id": "sub_later"})
	send("evt_old", "customer.subscription.updated", sub)
	access(true)
	async := map[string]any{"customer": "cus_test", "payment_intent": "pi_async", "amount": 1500, "currency": "eur", "refunded": false}
	send("evt_async_charge", "charge.succeeded", async)
	if id, err := s.Refund(ctx, uid, "pi_async"); err != nil || id != "re_async" {
		t.Fatal(id, err)
	}
	access(true)
	async["refunded"] = true
	send("evt_charge_pending", "charge.refunded", async)
	access(true)
	pending.Store(false)
	send("evt_refund_settled", "refund.updated", map[string]any{"id": "re_async"})
	access(false)
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, "SELECT id FROM users WHERE id=$1 FOR UPDATE", uid); e != nil {
			return e
		}
		if err := s.CloseAccount(ctx, uid, tx); err != nil {
			return err
		}
		_, e := tx.Exec(ctx, "DELETE FROM users WHERE id=$1", uid)
		return e
	})
	if err == nil {
		t.Fatal("unreachable provider allowed local deletion")
	}
	var exists bool
	if err := db.QueryRow(ctx, "SELECT EXISTS(SELECT FROM users WHERE id=$1)", uid).Scan(&exists); err != nil || !exists {
		t.Fatal("account lost", err)
	}
	s.enabled = false
	if _, err := s.Checkout(ctx, uid, "member@example.fr", "autonomie"); !errors.Is(err, ErrDisabled) {
		t.Fatal(err)
	}
	if _, err := s.Plans(ctx); !errors.Is(err, ErrDisabled) {
		t.Fatal(err)
	}
}

func TestUpgradeUsesStripeConfirmationForExistingSubscription(t *testing.T) {
	db := testdatabase.Open(t)
	ctx := t.Context()
	var user string
	if err := db.QueryRow(ctx, "INSERT INTO users(email,role) VALUES('upgrade@example.fr','student') RETURNING id::text").Scan(&user); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "INSERT INTO billing_customers VALUES($1,'cus_member')", user); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "INSERT INTO entitlements(user_id,plan_key,source,valid_until,stripe_customer_id,stripe_subscription_id) VALUES($1,'base','stripe',now()+interval '1 day','cus_member','sub_current')", user); err != nil {
		t.Fatal(err)
	}
	var confirms atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/v1/subscriptions/sub_current":
			if _, err := fmt.Fprint(w, `{"id":"sub_current","customer":"cus_member","status":"active","items":{"data":[{"id":"si_old","price":{"id":"price_base"}}]}}`); err != nil {
				t.Error(err)
			}
		case r.Method == "POST" && r.URL.Path == "/v1/billing_portal/sessions":
			confirms.Add(1)
			if r.FormValue("flow_data[type]") != "subscription_update_confirm" || r.FormValue("flow_data[subscription_update_confirm][subscription]") != "sub_current" || r.FormValue("flow_data[subscription_update_confirm][items][0][id]") != "si_old" || r.FormValue("flow_data[subscription_update_confirm][items][0][price]") != "price_suivi" {
				t.Error("upgrade did not target selected plan and existing item")
			}
			if _, err := fmt.Fprint(w, `{"url":"https://billing.stripe.com/session/test"}`); err != nil {
				t.Error(err)
			}
		default:
			t.Errorf("unexpected provider mutation %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()
	s := Service{db: db, enabled: true, key: "sk_test", endpoint: provider.URL, client: provider.Client(), webURL: "https://prep.example", plans: []planConfig{{Key: "base", Tier: "autonomie", PriceID: "price_base"}, {Key: "coach", Tier: "suivi", PriceID: "price_suivi"}}}
	if _, err := s.Checkout(ctx, user, "upgrade@example.fr", "base"); !errors.Is(err, ErrActiveSubscription) {
		t.Fatal(err)
	}
	link, err := s.Checkout(ctx, user, "upgrade@example.fr", "coach")
	if err != nil || link != "https://billing.stripe.com/session/test" || confirms.Load() != 1 {
		t.Fatal(link, err)
	}
	var stored string
	if err = db.QueryRow(ctx, "SELECT plan_key FROM entitlements WHERE user_id=$1", user).Scan(&stored); err != nil || stored != "base" {
		t.Fatal("granted upgrade before webhook", stored, err)
	}
	if _, err = db.Exec(ctx, "UPDATE entitlements SET plan_key='coach' WHERE user_id=$1", user); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Checkout(ctx, user, "upgrade@example.fr", "base"); !errors.Is(err, ErrActiveSubscription) {
		t.Fatal("unsupported downgrade created checkout", err)
	}
}
