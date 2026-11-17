package storegoogle

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pierremarsotlyon1/crpe/backend/internal/storeprovider"
)

const packageID = "fr.prepa.tests"
const pushAccount = "publisher@fixture.iam.gserviceaccount.com"
const pushSubscription = "projects/fixture/subscriptions/play-push"
const pushAudience = "https://service.example.invalid/store/google"
const purchaseRef = "fixture.token:one"
const userKey = "obfuscated-user-account"

var fixtureKey = sync.OnceValues(func() (*rsa.PrivateKey, error) { return rsa.GenerateKey(rand.Reader, 2048) })
var pushKey = sync.OnceValues(func() (*rsa.PrivateKey, error) { return rsa.GenerateKey(rand.Reader, 2048) })

func encoded(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(t *testing.T, status int, v any) *http.Response {
	t.Helper()
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(encoded(t, v)))), Header: http.Header{"Cache-Control": {"public,max-age=600"}}}
}
func setup(t *testing.T) (*Client, map[string]string) {
	t.Helper()
	key, err := fixtureKey()
	if err != nil {
		t.Fatal(err)
	}
	data, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	account := map[string]string{"type": "service_account", "client_email": "reader@fixture.iam.gserviceaccount.com", "private_key_id": "fixture-key", "private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: data})), "token_uri": "https://ignored.example.invalid"}
	env := map[string]string{"GOOGLE_PLAY_PACKAGE_NAME": packageID, "GOOGLE_PLAY_SERVICE_ACCOUNT_JSON": string(encoded(t, account)), "GOOGLE_PLAY_ENVIRONMENT": "Sandbox", "GOOGLE_PLAY_PUSH_EMAIL": pushAccount, "GOOGLE_PLAY_PUSH_AUDIENCE": pushAudience, "GOOGLE_PLAY_PUSH_SUBSCRIPTION": pushSubscription}
	c, err := FromEnv(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	c.now = func() time.Time { return time.Date(2026, 11, 17, 20, 0, 0, 0, time.UTC) }
	return c, env
}
func sample(c *Client) map[string]any {
	return map[string]any{"kind": "androidpublisher#subscriptionPurchaseV2", "subscriptionState": "SUBSCRIPTION_STATE_ACTIVE", "acknowledgementState": "ACKNOWLEDGEMENT_STATE_PENDING", "linkedPurchaseToken": "previous-token", "testPurchase": map[string]any{}, "externalAccountIdentifiers": map[string]any{"obfuscatedExternalAccountId": userKey}, "lineItems": []map[string]any{{"productId": "suivi", "expiryTime": c.now().Add(time.Hour).Format(time.RFC3339Nano), "offerDetails": map[string]any{"basePlanId": "mensuel"}, "autoRenewingPlan": map[string]any{"autoRenewEnabled": true}}}}
}
func checkAssertion(t *testing.T, r *http.Request, c *Client) {
	t.Helper()
	if r.Method != "POST" {
		t.Fatal("wrong OAuth method")
	}
	if err := r.ParseForm(); err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(r.Form.Get("assertion"), ".")
	if len(parts) != 3 || r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
		t.Fatal("invalid token assertion")
	}
	header, err := raw64.DecodeString(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	var meta jwtHeader
	if json.Unmarshal(header, &meta) != nil || meta.Kid != c.keyID || meta.Alg != "RS256" {
		t.Fatal("invalid assertion header")
	}
	data, _ := raw64.DecodeString(parts[1])
	var claims struct {
		Iss, Scope, Aud string
		Iat, Exp        int64
	}
	if json.Unmarshal(data, &claims) != nil || claims.Aud != oauthEndpoint || claims.Scope != androidScope || claims.Iss != c.email || claims.Iat != c.now().Unix() || claims.Exp-claims.Iat != 1800 {
		t.Fatal("invalid OAuth claims")
	}
	sig, _ := raw64.DecodeString(parts[2])
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(&c.signingKey.PublicKey, crypto.SHA256, digest[:], sig) != nil {
		t.Fatal("incorrect OAuth signature")
	}
}
func installPlay(t *testing.T, c *Client, payload map[string]any) (*int, *int) {
	t.Helper()
	auth, acks := 0, 0
	c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.String() {
		case oauthEndpoint:
			auth++
			checkAssertion(t, r, c)
			return response(t, 200, map[string]any{"access_token": "opaque", "token_type": "Bearer", "expires_in": 3600}), nil
		case publisherOrigin + packageID + "/purchases/subscriptionsv2/tokens/" + url.PathEscape(purchaseRef):
			if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer opaque" {
				t.Fatal("invalid subscription lookup")
			}
			return response(t, 200, payload), nil
		case publisherOrigin + packageID + "/purchases/subscriptions/suivi/tokens/" + url.PathEscape(purchaseRef) + ":acknowledge":
			if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer opaque" {
				t.Fatal("invalid acknowledgement")
			}
			acks++
			if _, ok := payload["outOfAppPurchaseContext"]; ok {
				var input struct {
					Account struct {
						ID string `json:"obfuscatedAccountId"`
					} `json:"externalAccountIds"`
				}
				if json.NewDecoder(r.Body).Decode(&input) != nil || input.Account.ID != userKey {
					t.Fatal("resubscription did not retain its verified account")
				}
				payload["externalAccountIdentifiers"] = map[string]string{"obfuscatedExternalAccountId": input.Account.ID}
				delete(payload, "outOfAppPurchaseContext")
			}
			payload["acknowledgementState"] = "ACKNOWLEDGEMENT_STATE_ACKNOWLEDGED"
			return response(t, 200, nil), nil
		default:
			t.Fatalf("unexpected outbound destination %s", r.URL.Hostname())
			return nil, nil
		}
	})
	return &auth, &acks
}
func TestVerifyAndAcknowledge(t *testing.T) {
	c, _ := setup(t)
	p := sample(c)
	auth, acks := installPlay(t, c, p)
	s, err := c.Verify(t.Context(), purchaseRef)
	if err != nil {
		t.Fatal(err)
	}
	if s.State != "active" || s.ProductID != "suivi" || s.BasePlanID != "mensuel" || s.AccountToken != userKey || s.Reference != purchaseRef || s.OriginalReference != purchaseRef || s.LinkedReference != "previous-token" || s.Environment != "Sandbox" || !s.AutoRenew || !s.NeedsAcknowledgement {
		t.Fatalf("unexpected normalized subscription: %#v", s)
	}
	other := s
	other.AccountToken = "different"
	if !errors.Is(c.Acknowledge(t.Context(), other), storeprovider.ErrInvalid) {
		t.Fatal("acknowledged wrong owner")
	}
	if err = c.Acknowledge(t.Context(), s); err != nil {
		t.Fatal(err)
	}
	if err = c.Acknowledge(t.Context(), s); err != nil {
		t.Fatal(err)
	}
	if *auth != 1 || *acks != 1 {
		t.Fatalf("OAuth calls=%d store acknowledgements=%d", *auth, *acks)
	}
}
func TestPurchaseStates(t *testing.T) {
	for _, row := range []struct {
		name, state string
		past        bool
		want        string
	}{
		{"active", "ACTIVE", false, "active"}, {"grace", "IN_GRACE_PERIOD", false, "active"}, {"cancelled paid through", "CANCELED", false, "active"}, {"cancelled expired", "CANCELED", true, "expired"}, {"active past expiry", "ACTIVE", true, "expired"}, {"pending", "PENDING", false, "pending"}, {"account hold", "ON_HOLD", false, "expired"}, {"paused", "PAUSED", false, "expired"}, {"expired", "EXPIRED", false, "expired"}, {"abandoned replacement", "PENDING_PURCHASE_CANCELED", false, "expired"},
	} {
		t.Run(row.name, func(t *testing.T) {
			c, _ := setup(t)
			p := sample(c)
			p["subscriptionState"] = "SUBSCRIPTION_STATE_" + row.state
			if row.past {
				p["lineItems"].([]map[string]any)[0]["expiryTime"] = c.now().Add(-time.Hour).Format(time.RFC3339)
			}
			installPlay(t, c, p)
			s, err := c.Verify(t.Context(), purchaseRef)
			if err != nil || s.State != row.want || s.NeedsAcknowledgement != (row.want == "active") {
				t.Fatalf("state=%q ack=%v err=%v", s.State, s.NeedsAcknowledgement, err)
			}
		})
	}
}
func TestRejectedPurchase(t *testing.T) {
	for _, row := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"production response on sandbox", func(p map[string]any) { delete(p, "testPurchase") }},
		{"ambiguous items", func(p map[string]any) {
			items := p["lineItems"].([]map[string]any)
			p["lineItems"] = append(items, items[0])
		}},
		{"unknown state", func(p map[string]any) { p["subscriptionState"] = "new_state" }},
		{"missing base plan", func(p map[string]any) { delete(p["lineItems"].([]map[string]any)[0], "offerDetails") }},
		{"active missing expiry", func(p map[string]any) { delete(p["lineItems"].([]map[string]any)[0], "expiryTime") }},
		{"unrecognized acknowledgement", func(p map[string]any) { p["acknowledgementState"] = "" }},
	} {
		t.Run(row.name, func(t *testing.T) {
			c, _ := setup(t)
			p := sample(c)
			row.change(p)
			installPlay(t, c, p)
			if _, err := c.Verify(t.Context(), purchaseRef); !errors.Is(err, storeprovider.ErrInvalid) {
				t.Fatalf("expected invalid purchase, got %v", err)
			}
		})
	}
	c, _ := setup(t)
	for _, s := range []string{"", "..", "x/y", "https://untrusted.invalid", "invalid?token"} {
		if _, err := c.Verify(t.Context(), s); !errors.Is(err, storeprovider.ErrInvalid) {
			t.Fatal("accepted invalid token")
		}
	}
}
func TestConfiguration(t *testing.T) {
	if c, err := FromEnv(func(string) string { return "" }); err != nil || c != nil {
		t.Fatal("empty configuration enabled")
	}
	_, env := setup(t)
	for key := range env {
		t.Run(key, func(t *testing.T) {
			if _, err := FromEnv(func(k string) string {
				if k == key {
					return ""
				}
				return env[k]
			}); err == nil {
				t.Fatal("partial configuration enabled")
			}
		})
	}
}
func signed(t *testing.T, key *rsa.PrivateKey, claims map[string]any, alg string) http.Header {
	t.Helper()
	head := encoded(t, map[string]string{"alg": alg, "kid": "push-key"})
	data := encoded(t, claims)
	prefix := raw64.EncodeToString(head) + "." + raw64.EncodeToString(data)
	hash := sha256.Sum256([]byte(prefix))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		t.Fatal(err)
	}
	return http.Header{"Authorization": {"Bearer " + prefix + "." + raw64.EncodeToString(sig)}}
}
func TestVerifiedNotification(t *testing.T) {
	c, _ := setup(t)
	key, err := pushKey()
	if err != nil {
		t.Fatal(err)
	}
	fetches := 0
	c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		fetches++
		if r.URL.String() != jwksEndpoint {
			t.Fatal("untrusted certificate source")
		}
		return response(t, 200, map[string]any{"keys": []map[string]string{{"kty": "RSA", "kid": "push-key", "alg": "RS256", "use": "sig", "n": raw64.EncodeToString(key.N.Bytes()), "e": "AQAB"}}}), nil
	})
	claims := func() map[string]any {
		return map[string]any{"sub": "service-account-id", "iss": "https://accounts.google.com", "aud": pushAudience, "email": pushAccount, "email_verified": true, "iat": c.now().Unix(), "exp": c.now().Add(time.Hour).Unix()}
	}
	envelope := func(pkg, sub string, event map[string]any) []byte {
		e := map[string]any{"version": "1.0", "packageName": pkg}
		for k, v := range event {
			e[k] = v
		}
		return encoded(t, map[string]any{"subscription": sub, "message": map[string]any{"messageId": "message-id", "data": base64.StdEncoding.EncodeToString(encoded(t, e))}})
	}
	live := map[string]any{"subscriptionNotification": map[string]string{"purchaseToken": purchaseRef}}
	body := envelope(packageID, pushSubscription, live)
	headers := signed(t, key, claims(), "RS256")
	for _, row := range []struct {
		name string
		data []byte
		ref  string
	}{
		{"subscription", body, purchaseRef}, {"test", envelope(packageID, pushSubscription, map[string]any{"testNotification": map[string]string{}}), ""}, {"voided subscription", envelope(packageID, pushSubscription, map[string]any{"voidedPurchaseNotification": map[string]any{"purchaseToken": purchaseRef, "productType": 1}}), purchaseRef},
	} {
		t.Run(row.name, func(t *testing.T) {
			notice, err := c.Notification(context.Background(), headers, row.data)
			if err != nil || notice.Reference != row.ref || notice.EventID != "message-id" {
				t.Fatalf("notice=%#v err=%v", notice, err)
			}
		})
	}
	if fetches != 1 {
		t.Fatalf("certificate fetches=%d", fetches)
	}
	for _, row := range []struct {
		name, key string
		value     any
	}{{"audience", "aud", "wrong"}, {"service account", "email", "other@example.invalid"}, {"unverified", "email_verified", false}, {"issuer", "iss", "https://untrusted.invalid"}, {"expired", "exp", c.now().Unix()}, {"future", "iat", c.now().Add(time.Hour).Unix()}} {
		t.Run(row.name, func(t *testing.T) {
			v := claims()
			v[row.key] = row.value
			if _, err := c.Notification(t.Context(), signed(t, key, v, "RS256"), body); !errors.Is(err, storeprovider.ErrInvalid) {
				t.Fatal("untrusted push accepted")
			}
		})
	}
	other, _ := fixtureKey()
	for _, row := range []struct {
		name    string
		headers http.Header
		body    []byte
	}{{"wrong signature", signed(t, other, claims(), "RS256"), body}, {"unsupported algorithm", signed(t, key, claims(), "HS256"), body}, {"wrong project subscription", headers, envelope(packageID, "projects/another/subscriptions/play-push", live)}, {"wrong package", headers, envelope("fr.wrong.app", pushSubscription, live)}, {"ambiguous event", headers, envelope(packageID, pushSubscription, map[string]any{"testNotification": map[string]string{}, "subscriptionNotification": map[string]string{"purchaseToken": purchaseRef}})}} {
		t.Run(row.name, func(t *testing.T) {
			if _, err := c.Notification(t.Context(), row.headers, row.body); !errors.Is(err, storeprovider.ErrInvalid) {
				t.Fatal("untrusted notification accepted")
			}
		})
	}
}

func TestDeferredReplacementEntitlement(t *testing.T) {
	for _, row := range []struct {
		name              string
		before, after     int
		deferred, reverse bool
		want              string
	}{
		{"before effective date", 48, 0, true, false, "suivi"}, {"before in reverse order", 48, 0, true, true, "suivi"},
		{"expired old not yet replaced", -1, 0, true, false, "suivi"}, {"after transfer", -1, 48, false, false, "autonomie"},
		{"expired both", -48, -1, false, true, "autonomie"}, {"two live plans", 12, 48, false, false, ""},
		{"deferred future line already active", 24, 48, true, false, ""},
	} {
		t.Run(row.name, func(t *testing.T) {
			c, _ := setup(t)
			p := sample(c)
			old := p["lineItems"].([]map[string]any)[0]
			old["expiryTime"] = c.now().Add(time.Duration(row.before) * time.Hour).Format(time.RFC3339)
			next := map[string]any{"productId": "autonomie", "offerDetails": map[string]any{"basePlanId": "mensuel"}}
			if row.after != 0 {
				next["expiryTime"] = c.now().Add(time.Duration(row.after) * time.Hour).Format(time.RFC3339)
			}
			if row.deferred {
				old["deferredItemReplacement"] = map[string]string{"productId": "autonomie"}
			} else {
				next["itemReplacement"] = map[string]string{"productId": "suivi", "basePlanId": "mensuel", "replacementMode": "DEFERRED"}
			}
			if row.reverse {
				p["lineItems"] = []map[string]any{next, old}
			} else {
				p["lineItems"] = []map[string]any{old, next}
			}
			installPlay(t, c, p)
			s, err := c.Verify(t.Context(), purchaseRef)
			if row.want == "" {
				if !errors.Is(err, storeprovider.ErrInvalid) {
					t.Fatal("accepted ambiguous products")
				}
				return
			}
			if err != nil || s.ProductID != row.want {
				t.Fatalf("product=%q err=%v", s.ProductID, err)
			}
			if row.before < 0 && row.after < 1 && s.State == "active" {
				t.Fatal("granted an expired period")
			}
		})
	}
}

func TestOutOfAppResubscription(t *testing.T) {
	c, _ := setup(t)
	p := sample(c)
	delete(p, "externalAccountIdentifiers")
	p["outOfAppPurchaseContext"] = map[string]any{"expiredExternalAccountIdentifiers": map[string]string{"obfuscatedExternalAccountId": userKey}, "expiredPurchaseToken": "old-expired-fixture-token"}
	_, acks := installPlay(t, c, p)
	before, err := c.Verify(t.Context(), purchaseRef)
	if err != nil || before.AccountToken != userKey {
		t.Fatalf("restore failed: %v", err)
	}
	if err = c.Acknowledge(t.Context(), before); err != nil || *acks != 1 {
		t.Fatalf("ack failed: %v", err)
	}
	after, err := c.Verify(t.Context(), purchaseRef)
	if err != nil || after.AccountToken != before.AccountToken || after.NeedsAcknowledgement {
		t.Fatalf("owner lost after acknowledgement: %v", err)
	}
}

func TestAcknowledgementRequiresCommittedEntitlement(t *testing.T) {
	for _, row := range []struct {
		name   string
		change func(map[string]any, *Client)
	}{
		{"revocation", func(p map[string]any, _ *Client) { p["subscriptionState"] = "SUBSCRIPTION_STATE_EXPIRED" }},
		{"account hold", func(p map[string]any, _ *Client) { p["subscriptionState"] = "SUBSCRIPTION_STATE_ON_HOLD" }},
		{"product switch", func(p map[string]any, _ *Client) { p["lineItems"].([]map[string]any)[0]["productId"] = "autonomie" }},
		{"base plan change", func(p map[string]any, _ *Client) {
			p["lineItems"].([]map[string]any)[0]["offerDetails"] = map[string]string{"basePlanId": "annuel"}
		}},
		{"renewed period", func(p map[string]any, c *Client) {
			p["lineItems"].([]map[string]any)[0]["expiryTime"] = c.now().Add(10 * time.Hour).Format(time.RFC3339)
		}},
	} {
		t.Run(row.name, func(t *testing.T) {
			c, _ := setup(t)
			p := sample(c)
			_, acks := installPlay(t, c, p)
			verified, err := c.Verify(t.Context(), purchaseRef)
			if err != nil {
				t.Fatal(err)
			}
			row.change(p, c)
			p["acknowledgementState"] = "ACKNOWLEDGEMENT_STATE_ACKNOWLEDGED"
			// Even an already acknowledged store token cannot turn a stale ledger snapshot into success.
			if err = c.Acknowledge(t.Context(), verified); !errors.Is(err, storeprovider.ErrUnavailable) || *acks != 0 {
				t.Fatalf("missing reconciliation request: err=%v acknowledges=%d", err, *acks)
			}
		})
	}
}
