//go:build integration

package learning

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pierremarsotlyon1/crpe/backend/internal/billing"
	"github.com/pierremarsotlyon1/crpe/backend/internal/storebilling"
	"github.com/pierremarsotlyon1/crpe/backend/internal/storeprovider"
	"github.com/pierremarsotlyon1/crpe/backend/internal/testdatabase"
)

type verifiedStoreFixture struct{ snapshot storeprovider.Snapshot }

func (p *verifiedStoreFixture) Verify(context.Context, string) (storeprovider.Snapshot, error) {
	return p.snapshot, nil
}
func (*verifiedStoreFixture) Acknowledge(context.Context, storeprovider.Snapshot) error { return nil }
func (*verifiedStoreFixture) Notification(context.Context, http.Header, []byte) (storeprovider.Notification, error) {
	return storeprovider.Notification{}, storeprovider.ErrInvalid
}

func TestNativeReceiptHTTPAndAccountDeletion(t *testing.T) {
	db := testdatabase.Open(t)
	env := map[string]string{"SALES_ENABLED": "true", "STORE_PRODUCTS_JSON": `[{"platform":"apple","productId":"support.monthly","tier":"suivi","planKey":"support"}]`, "APPLE_STORE_ENVIRONMENT": "Sandbox", "STORE_REFERENCE_KEY": base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))}
	get := func(k string) string { return env[k] }
	apple := new(verifiedStoreFixture)
	stores, e := storebilling.New(db, get, map[string]storeprovider.Provider{"apple": apple})
	if e != nil {
		t.Fatal(e)
	}
	stripe, e := billing.New(db, get)
	if e != nil {
		t.Fatal(e)
	}
	mail := new(mailbox)
	app, e := New(db, mail, nil, Options{Stores: stores, Billing: stripe, SalesEnabled: true, WebURL: "http://localhost:3000", MobileURL: "crpemaths://auth"}, slog.Default())
	if e != nil {
		t.Fatal(e)
	}
	mux := http.NewServeMux()
	app.Register(mux)
	guest := browser{t: t, h: mux}
	guest.req("GET", "/v1/me/store-products?platform=apple", "", 401)
	login := func(email string) browser {
		guest.req("POST", "/v1/auth/request", fmt.Sprintf(`{"email":%q,"client":"mobile"}`, email), 202)
		_, token, _ := strings.Cut(mail.body, "#token=")
		r := guest.req("POST", "/v1/auth/verify", fmt.Sprintf(`{"client":"mobile","token":%q}`, token), 200)
		return browser{t: t, h: mux, token: r["token"].(string)}
	}
	a, b := login("native-a@example.test"), login("native-b@example.test")
	catalog := a.req("GET", "/v1/me/store-products?platform=apple", "", 200)
	apple.snapshot = storeprovider.Snapshot{Reference: "31313", OriginalReference: "31313", AccountToken: catalog["accountToken"].(string), Environment: "Sandbox", ProductID: "support.monthly", State: "active", ObservedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
	order := a.req("POST", "/v1/me/store-orders", `{"platform":"apple","productId":"support.monthly"}`, 200)
	a.req("DELETE", "/v1/me/store-orders/apple?orderId="+order["id"].(string), "", 204)
	receipt := `{"platform":"apple","productId":"support.monthly","reference":"31313"}`
	b.req("POST", "/v1/me/store-purchases", receipt, 403)
	a.req("GET", "/v1/support", "", 403)
	a.req("POST", "/v1/me/store-purchases", receipt, 200)
	a.req("GET", "/v1/support", "", 200)
	access := a.req("GET", "/v1/me/access", "", 200)
	if access["tier"] != "suivi" || access["validUntil"] == nil || len(access["providers"].([]any)) != 1 {
		t.Fatal(access)
	}
	b.req("GET", "/v1/support", "", 403)
	exported := a.req("GET", "/v1/me/export", "", 200)
	raw, _ := json.Marshal(exported)
	if strings.Contains(string(raw), "31313") || !strings.Contains(string(raw), `"provider":"apple"`) {
		t.Fatal("export store data")
	}
	a.req("DELETE", "/v1/me", "", 204)
	var erased bool
	if e = db.QueryRow(t.Context(), "SELECT user_id IS NULL AND length(lookup_cipher)=0 FROM store_memberships").Scan(&erased); e != nil || !erased {
		t.Fatal("store purchase tombstone", e)
	}
}
