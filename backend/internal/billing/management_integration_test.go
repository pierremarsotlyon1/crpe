//go:build integration

package billing

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/pierremarsotlyon1/crpe/backend/internal/testdatabase"
)

func TestExistingStripeManagementAfterSalesClose(t *testing.T) {
	db := testdatabase.Open(t)
	ctx := t.Context()
	var user string
	if e := db.QueryRow(ctx, `INSERT INTO users(email,role)VALUES('stripe-legacy@example.test','student') RETURNING id::text`).Scan(&user); e != nil {
		t.Fatal(e)
	}
	s, e := New(db, func(string) string { return "" })
	if e != nil {
		t.Fatal(e)
	}
	if list, e := s.Invoices(ctx, user); e != nil || len(list) != 0 {
		t.Fatal(list, e)
	}
	if _, e = db.Exec(ctx, "INSERT INTO billing_customers VALUES($1,'cus_legacy')", user); e != nil {
		t.Fatal(e)
	}
	s.key = "test"
	active := true
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/billing_portal/sessions":
			if r.FormValue("customer") != "cus_legacy" {
				t.Error("wrong member")
			}
			_, _ = fmt.Fprint(w, `{"url":"https://billing.stripe.com/p/session/fixture"}`)
		case "/v1/invoices":
			_, _ = fmt.Fprint(w, `{"data":[{"id":"in_fixture","status":"paid","currency":"eur","amount_paid":3000,"created":1}]}`)
		case "/v1/checkout/sessions":
			_, _ = fmt.Fprint(w, `{"data":[]}`)
		case "/v1/subscriptions":
			if active {
				_, _ = fmt.Fprint(w, `{"data":[{"customer":"cus_legacy","status":"active"}]}`)
			} else {
				_, _ = fmt.Fprint(w, `{"data":[]}`)
			}
		default:
			t.Error("unexpected Stripe call", r.URL.Path)
			w.WriteHeader(400)
		}
	}))
	defer fixture.Close()
	s.endpoint = fixture.URL
	if url, e := s.Portal(ctx, user); e != nil || url == "" {
		t.Fatal(url, e)
	}
	if invoices, e := s.Invoices(ctx, user); e != nil || len(invoices) != 1 {
		t.Fatal(invoices, e)
	}
	if _, e = s.Checkout(ctx, user, "stripe-legacy@example.test", "plan"); !errors.Is(e, ErrDisabled) {
		t.Fatal(e)
	}
	check := func() error {
		return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error { return s.CheckNativeOrder(ctx, tx, user) })
	}
	if e = check(); !errors.Is(e, ErrActiveSubscription) {
		t.Fatal(e)
	}
	active = false
	if e = check(); e != nil {
		t.Fatal(e)
	}
}
