//go:build integration

package storeconfig

import (
	"github.com/pierremarsotlyon1/crpe/backend/internal/testdatabase"
	"sync"
	"testing"
)

func TestEnvironmentBindingAndConcurrentInitialStartup(t *testing.T) {
	db := testdatabase.Open(t)
	ctx := t.Context()
	if _, e := environment(func(k string) string {
		return map[string]string{"APPLE_STORE_ENVIRONMENT": "Sandbox", "GOOGLE_PLAY_ENVIRONMENT": "Production"}[k]
	}); e == nil {
		t.Fatal("mixed stores accepted")
	}
	if _, e := environment(func(k string) string {
		return map[string]string{"STRIPE_SECRET_KEY": "sk_live_fixture", "APPLE_STORE_ENVIRONMENT": "Sandbox"}[k]
	}); e == nil {
		t.Fatal("live Stripe with test store")
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, env := range []string{"Production", "Sandbox"} {
		wg.Go(func() { results <- bind(ctx, db, env) })
	}
	wg.Wait()
	close(results)
	ok := 0
	for e := range results {
		if e == nil {
			ok++
		}
	}
	if ok != 1 {
		t.Fatal("expected one environment to win", ok)
	}
	var stored string
	if e := db.QueryRow(ctx, "SELECT environment FROM store_deployment").Scan(&stored); e != nil {
		t.Fatal(e)
	}
	other := "Sandbox"
	if stored == "Sandbox" {
		other = "Production"
	}
	if _, e := New(ctx, db, func(k string) string {
		if k == "STORE_ENVIRONMENT" {
			return other
		}
		return ""
	}); e == nil {
		t.Fatal("sales-off deployment changed environment")
	}
	if _, e := db.Exec(ctx, `INSERT INTO store_memberships(platform,environment,original_hash,lookup_cipher,plan_key,product_id,base_plan_id,tier,state,observed_at,auto_renew) VALUES('google',$1,'x','y','p','p','month','autonomie','expired',now(),false)`, other); e != nil {
		t.Fatal(e)
	}
	if e := bind(ctx, db, stored); e == nil {
		t.Fatal("mixed pre-existing ledger accepted")
	}
}
