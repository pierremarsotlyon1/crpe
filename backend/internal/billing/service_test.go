package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSalesDisabledNeverCallsProvider(t *testing.T) {
	s, err := New(nil, func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Plans(t.Context()); !errors.Is(err, ErrDisabled) {
		t.Fatal(err)
	}
	if _, err = s.Checkout(t.Context(), "user", "email", "plan"); !errors.Is(err, ErrDisabled) {
		t.Fatal(err)
	}
	if _, err = s.Portal(t.Context(), "user"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err = s.Refund(t.Context(), "admin", "payment"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}

func TestPlanConfigurationRejectsAmbiguousPriceEntitlements(t *testing.T) {
	for _, tc := range []struct {
		name, plans string
		valid       bool
	}{
		{"base default", `[{"key":"base","name":"Autonomie","priceId":"price_base"}]`, true},
		{"explicit support", `[{"key":"coach","name":"Suivi","priceId":"price_coach","tier":"suivi"}]`, true},
		{"unknown tier", `[{"key":"coach","name":"Suivi","priceId":"price_coach","tier":"unknown"}]`, false},
		{"shared price", `[{"key":"base","name":"Autonomie","priceId":"price_shared"},{"key":"coach","name":"Suivi","priceId":"price_shared","tier":"suivi"}]`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := New(nil, func(key string) string {
				if key == "BILLING_PLANS_JSON" {
					return tc.plans
				}
				return ""
			})
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
			if tc.name == "base default" && s.plans[0].Tier != "autonomie" {
				t.Fatal("omitted tier granted premium access")
			}
		})
	}
}

func TestPlanPricesComeFromProviderAndRequireRecurringInterval(t *testing.T) {
	for _, interval := range []string{"month", "year", "week"} {
		t.Run(interval, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				user, _, ok := r.BasicAuth()
				if !ok || user != "sk_test_fixture" {
					t.Error("provider credentials missing")
				}
				if r.URL.Path != "/v1/prices/price_test" {
					t.Errorf("unexpected endpoint %s", r.URL.Path)
				}
				if _, err := fmt.Fprintf(w, `{"active":true,"currency":"eur","unit_amount":1900,"recurring":{"interval":%q,"interval_count":1}}`, interval); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			s := &Service{enabled: true, key: "sk_test_fixture", client: server.Client(), endpoint: server.URL, plans: []planConfig{{Key: "autonomie", Name: "Autonomie", PriceID: "price_test"}}}
			plans, err := s.Plans(t.Context())
			if interval == "week" {
				if err == nil {
					t.Fatal("unsupported plan accepted")
				}
				return
			}
			if err != nil || len(plans) != 1 || plans[0].Amount != 1900 || plans[0].Interval != interval {
				t.Fatalf("plans=%+v error=%v", plans, err)
			}
		})
	}
}

func TestWebhookRejectsTamperingExpiredAndFutureSignatures(t *testing.T) {
	secret := "whsec_test_fixture"
	body := []byte(`{"id":"evt_test","type":"test"}`)
	now := time.Now()
	for _, test := range []struct {
		name string
		time time.Time
		body []byte
		want bool
	}{
		{"valid", now, body, true},
		{"tampered", now, []byte(`{"id":"evt_altered"}`), false},
		{"expired", now.Add(-6 * time.Minute), body, false},
		{"future", now.Add(6 * time.Minute), body, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			prefix := fmt.Sprintf("%d", test.time.Unix())
			mac := hmac.New(sha256.New, []byte(secret))
			mac.Write([]byte(prefix + "."))
			mac.Write(body)
			sig := "t=" + prefix + ",v1=" + hex.EncodeToString(mac.Sum(nil))
			if valid := verifySignature(test.body, sig, secret, now) == nil; valid != test.want {
				t.Fatalf("valid=%v", valid)
			}
		})
	}
}

func TestProviderErrorsDoNotLeakResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		if _, err := fmt.Fprint(w, "secret customer data"); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	s := &Service{endpoint: server.URL, client: server.Client(), key: "test"}
	err := s.request(context.Background(), http.MethodGet, "/v1/prices", nil, "", new(any))
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("error leaks data: %v", err)
	}
}
