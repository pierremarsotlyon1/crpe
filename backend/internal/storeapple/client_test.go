package storeapple

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pierremarsotlyon1/crpe/backend/internal/storeprovider"
)

const memberID = "550e8400-e29b-41d4-a716-446655440000"

type fixture struct {
	client    *Client
	chain     []string
	signer    *ecdsa.PrivateKey
	issuer    *x509.Certificate
	issuerKey *ecdsa.PrivateKey
	now       time.Time
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	f := fixture{now: time.Now().UTC().Truncate(time.Second)}
	var parent *x509.Certificate
	var parentKey *ecdsa.PrivateKey
	var root *x509.Certificate
	for i := range 3 {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		cert := &x509.Certificate{SerialNumber: big.NewInt(int64(i + 1)), Subject: pkix.Name{CommonName: "isolated Apple test fixture"}, NotBefore: f.now.Add(-time.Hour), NotAfter: f.now.Add(24 * time.Hour), BasicConstraintsValid: true, IsCA: i != 2, KeyUsage: x509.KeyUsageDigitalSignature}
		if i != 2 {
			cert.KeyUsage |= x509.KeyUsageCertSign
		}
		if i == 1 {
			cert.ExtraExtensions = []pkix.Extension{{Id: asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 2, 1}, Value: []byte{5, 0}}}
		}
		if i == 2 {
			cert.ExtraExtensions = []pkix.Extension{{Id: asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 11, 1}, Value: []byte{5, 0}}}
		}
		if parent == nil {
			parent, parentKey = cert, key
		}
		der, err := x509.CreateCertificate(rand.Reader, cert, parent, &key.PublicKey, parentKey)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := x509.ParseCertificate(der)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			root = parsed
		}
		if i == 1 {
			f.issuer, f.issuerKey = parsed, key
		}
		parent, parentKey = parsed, key
		f.chain = append([]string{base64.StdEncoding.EncodeToString(der)}, f.chain...)
		f.signer = key
	}
	der, err := x509.MarshalPKCS8PrivateKey(f.signer)
	if err != nil {
		t.Fatal(err)
	}
	settings := map[string]string{"APPLE_STORE_BUNDLE_ID": "fr.test.crpe", "APPLE_STORE_APP_ID": "42", "APPLE_STORE_KEY_ID": "TESTKEY", "APPLE_STORE_ISSUER_ID": memberID, "APPLE_STORE_PRIVATE_KEY": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), "APPLE_STORE_ENVIRONMENT": "Sandbox"}
	f.client, err = FromEnv(func(s string) string { return settings[s] })
	if err != nil {
		t.Fatal(err)
	}
	f.client.now = func() time.Time { return f.now }
	f.client.roots = x509.NewCertPool()
	f.client.roots.AddCert(root)
	return f
}

func (f fixture) signed(t *testing.T, payload any) string {
	t.Helper()
	header, err := json.Marshal(map[string]any{"alg": "ES256", "x5c": f.chain})
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	message := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(body)
	digest := sha256.Sum256([]byte(message))
	r, s, err := ecdsa.Sign(rand.Reader, f.signer, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return message + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func (f fixture) purchase() transaction {
	return transaction{TransactionID: "200", OriginalTransactionID: "100", AccountToken: memberID, BundleID: f.client.bundle, ProductID: "suivi.monthly", Environment: "Sandbox", Type: "Auto-Renewable Subscription", Ownership: "PURCHASED", ExpiresDate: f.now.Add(time.Hour).UnixMilli(), SignedDate: f.now.UnixMilli()}
}

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func (f fixture) responses(t *testing.T, latest transaction, status int, grace int64) {
	t.Helper()
	old := f.purchase()
	old.TransactionID, old.ProductID = "100", "autonomie.monthly"
	renewal := map[string]any{"originalTransactionId": "100", "environment": latest.Environment, "productId": latest.ProductID, "autoRenewStatus": 1, "signedDate": f.now.UnixMilli(), "gracePeriodExpiresDate": grace}
	signedOld, signedLatest, signedRenewal := f.signed(t, old), f.signed(t, latest), f.signed(t, renewal)
	f.client.client.Transport = transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Scheme != "https" || r.URL.Host != "api.storekit-sandbox.apple.com" || r.Method != http.MethodGet {
			t.Error("provider endpoint or method changed")
		}
		f.assertCredential(t, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		var body any
		switch r.URL.Path {
		case "/inApps/v1/transactions/100":
			body = map[string]any{"signedTransactionInfo": signedOld}
		case "/inApps/v1/subscriptions/100":
			body = map[string]any{"bundleId": f.client.bundle, "environment": "Sandbox", "appAppleId": 42, "data": []any{map[string]any{"lastTransactions": []any{map[string]any{"originalTransactionId": "100", "status": status, "signedTransactionInfo": signedLatest, "signedRenewalInfo": signedRenewal}}}}}
		default:
			t.Errorf("unexpected provider path %s", r.URL.Path)
		}
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
	})
}

func (f fixture) assertCredential(t *testing.T, token string) {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatal("missing server authentication")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sig) != 64 {
		t.Fatal("invalid server signature")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(&f.signer.PublicKey, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Fatal("server credential was not signed by configured key")
	}
	bytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims struct {
		Issuer, Audience, Bundle string
		Issued, Expires          int64
	}
	var parsed map[string]any
	if err := json.Unmarshal(bytes, &parsed); err != nil {
		t.Fatal(err)
	}
	claims.Issuer, _ = parsed["iss"].(string)
	claims.Audience, _ = parsed["aud"].(string)
	claims.Bundle, _ = parsed["bid"].(string)
	claims.Issued = int64(parsed["iat"].(float64))
	claims.Expires = int64(parsed["exp"].(float64))
	if claims.Issuer != memberID || claims.Audience != "appstoreconnect-v1" || claims.Bundle != f.client.bundle || claims.Issued != f.now.Unix() || claims.Expires-claims.Issued != 300 {
		t.Fatal("wrong App Store JWT claims")
	}
}

func TestVerifyRefreshesOriginalSubscription(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		status int
		edit   func(*transaction, time.Time)
		grace  bool
		want   string
	}{
		{name: "upgrade uses current product", status: 1, want: "active"},
		{name: "expired", status: 2, want: "expired"},
		{name: "billing retry has no access", status: 3, want: "expired"},
		{name: "grace grants only grace period", status: 4, grace: true, edit: func(t *transaction, now time.Time) { t.ExpiresDate = now.Add(-time.Hour).UnixMilli() }, want: "active"},
		{name: "revoked old receipt cannot restore", status: 5, want: "revoked"},
		{name: "revocation overrides active state", status: 1, edit: func(t *transaction, now time.Time) { t.RevocationDate = now.UnixMilli() }, want: "revoked"},
		{name: "past expiry fails closed", status: 1, edit: func(t *transaction, now time.Time) { t.ExpiresDate = now.Add(-time.Second).UnixMilli() }, want: "expired"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			latest := f.purchase()
			if tc.edit != nil {
				tc.edit(&latest, f.now)
			}
			grace := int64(0)
			if tc.grace {
				grace = f.now.Add(30 * time.Minute).UnixMilli()
			}
			f.responses(t, latest, tc.status, grace)
			got, err := f.client.Verify(t.Context(), "100")
			if err != nil || got.State != tc.want || got.ProductID != "suivi.monthly" || got.AccountToken != memberID || got.Reference != "200" || got.OriginalReference != "100" || got.NeedsAcknowledgement {
				t.Fatalf("unexpected verified purchase: %+v, %v", got, err)
			}
			if tc.grace && got.ExpiresAt.UnixMilli() != grace {
				t.Fatal("grace expiration not respected")
			}
		})
	}
}

func TestRejectsInvalidTransactions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		edit func(*transaction)
	}{
		{"different app", func(t *transaction) { t.BundleID = "another.app" }},
		{"different environment", func(t *transaction) { t.Environment = "Production" }},
		{"local StoreKit", func(t *transaction) { t.Environment = "Xcode" }},
		{"different owner", func(t *transaction) { t.AccountToken = "650e8400-e29b-41d4-a716-446655440000" }},
		{"missing owner", func(t *transaction) { t.AccountToken = "" }},
		{"family sharing not enabled", func(t *transaction) { t.Ownership = "FAMILY_SHARED" }},
		{"not a subscription", func(t *transaction) { t.Type = "Consumable" }},
		{"different original transaction", func(t *transaction) { t.OriginalTransactionID = "999" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			latest := f.purchase()
			tc.edit(&latest)
			f.responses(t, latest, 1, 0)
			if _, err := f.client.Verify(t.Context(), "100"); !errors.Is(err, storeprovider.ErrInvalid) {
				t.Fatalf("invalid purchase accepted: %v", err)
			}
		})
	}
}

func TestNotificationsAuthenticateThenRequestRefresh(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	payload := map[string]any{"notificationUUID": memberID, "notificationType": "REFUND", "version": "2.0", "signedDate": f.now.UnixMilli(), "data": map[string]any{"bundleId": f.client.bundle, "environment": "Sandbox", "appAppleId": 42, "signedTransactionInfo": f.signed(t, f.purchase())}}
	body, err := json.Marshal(map[string]string{"signedPayload": f.signed(t, payload)})
	if err != nil {
		t.Fatal(err)
	}
	n, err := f.client.Notification(t.Context(), nil, body)
	if err != nil || n.Reference != "100" || n.EventID != memberID {
		t.Fatalf("valid refund did not trigger current-state refresh: %+v %v", n, err)
	}
	// An attacker can generate its own entire chain with the correct Apple OIDs.
	// Supplying that root in x5c must not establish trust.
	attacker := newFixture(t)
	forged, err := json.Marshal(map[string]string{"signedPayload": attacker.signed(t, payload)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.client.Notification(t.Context(), nil, forged); !errors.Is(err, storeprovider.ErrInvalid) {
		t.Fatal("untrusted signing chain accepted")
	}
	payload["notificationType"] = "TEST"
	body, err = json.Marshal(map[string]string{"signedPayload": f.signed(t, payload)})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := f.client.Notification(t.Context(), nil, body); err != nil || n.Reference != "" {
		t.Fatalf("authenticated provider test failed: %+v %v", n, err)
	}
}

func TestRejectsTamperedSignatures(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	valid := f.signed(t, f.purchase())
	parts := strings.Split(valid, ".")
	other := f.purchase()
	other.AccountToken = "650e8400-e29b-41d4-a716-446655440000"
	otherBytes, err := json.Marshal(other)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, value string }{
		{"modified payload", parts[0] + "." + base64.RawURLEncoding.EncodeToString(otherBytes) + "." + parts[2]},
		{"alg none", base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`)) + "." + parts[1] + "."},
		{"invalid signature length", parts[0] + "." + parts[1] + ".AA"},
		{"oversized", strings.Repeat("a", 256<<10+1)},
		{"malformed", "not-a-jws"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := f.client.decodeTransaction(tc.value); !errors.Is(err, storeprovider.ErrInvalid) {
				t.Fatal("forged signed transaction accepted")
			}
		})
	}
}

func TestAuthenticatedRenewalSummary(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, kind, subtype, bundle string
		wantError                   bool
	}{
		{"aggregate extension", "RENEWAL_EXTENSION", "SUMMARY", "fr.test.crpe", false},
		{"wrong notification type", "DID_RENEW", "SUMMARY", "fr.test.crpe", true},
		{"missing summary subtype", "RENEWAL_EXTENSION", "", "fr.test.crpe", true},
		{"other app", "RENEWAL_EXTENSION", "SUMMARY", "other.app", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			payload := map[string]any{"notificationUUID": memberID, "notificationType": tc.kind, "subtype": tc.subtype, "version": "2.0", "signedDate": f.now.UnixMilli(), "summary": map[string]any{"bundleId": tc.bundle, "environment": "Sandbox", "appAppleId": 42}}
			body, err := json.Marshal(map[string]string{"signedPayload": f.signed(t, payload)})
			if err != nil {
				t.Fatal(err)
			}
			n, err := f.client.Notification(t.Context(), nil, body)
			if tc.wantError {
				if !errors.Is(err, storeprovider.ErrInvalid) {
					t.Fatal("out-of-scope summary accepted")
				}
			} else if err != nil || n.Reference != "" || n.EventID != memberID {
				t.Fatalf("valid summary rejected or invented a transaction: %+v %v", n, err)
			}
		})
	}
}

func TestSigningProfileAndCertificateDates(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		edit func(*transaction, *fixture)
	}{
		{"signed before certificate validity", func(p *transaction, f *fixture) { p.SignedDate = f.now.Add(-2 * time.Hour).UnixMilli() }},
		{"future signed date", func(p *transaction, f *fixture) { p.SignedDate = f.now.Add(10 * time.Minute).UnixMilli() }},
		{"missing signed date", func(p *transaction, _ *fixture) { p.SignedDate = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			p := f.purchase()
			tc.edit(&p, &f)
			if _, err := f.client.decodeTransaction(f.signed(t, p)); !errors.Is(err, storeprovider.ErrInvalid) {
				t.Fatal("invalid certificate date accepted")
			}
		})
	}
	t.Run("trusted certificate without App Store signing profile", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		cert := &x509.Certificate{SerialNumber: big.NewInt(99), Subject: pkix.Name{CommonName: "different purpose"}, NotBefore: f.now.Add(-time.Hour), NotAfter: f.now.Add(time.Hour), BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature}
		der, err := x509.CreateCertificate(rand.Reader, cert, f.issuer, &f.signer.PublicKey, f.issuerKey)
		if err != nil {
			t.Fatal(err)
		}
		f.chain[0] = base64.StdEncoding.EncodeToString(der)
		if _, err := f.client.decodeTransaction(f.signed(t, f.purchase())); !errors.Is(err, storeprovider.ErrInvalid) {
			t.Fatal("certificate for another purpose accepted")
		}
	})
}

func TestConfigurationAndTransportFailClosed(t *testing.T) {
	t.Parallel()
	if client, err := FromEnv(func(string) string { return "" }); err != nil || client != nil {
		t.Fatal("missing provider must remain disabled")
	}
	if _, err := FromEnv(func(s string) string {
		if s == "APPLE_STORE_BUNDLE_ID" {
			return "fr.test.crpe"
		}
		return ""
	}); err == nil {
		t.Fatal("partial provider configuration accepted")
	}
	f := newFixture(t)
	for _, status := range []int{http.StatusUnauthorized, http.StatusTooManyRequests, http.StatusTemporaryRedirect, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			f.client.client.Transport = transport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("private provider detail"))}, nil
			})
			if _, err := f.client.Verify(context.Background(), "100"); !errors.Is(err, storeprovider.ErrUnavailable) {
				t.Fatalf("wrong error classification: %v", err)
			}
		})
	}
	for _, reference := range []string{"", "../100", "100?status=1", strings.Repeat("1", 65)} {
		if _, err := f.client.Verify(t.Context(), reference); !errors.Is(err, storeprovider.ErrInvalid) {
			t.Fatal("invalid transaction ID accepted")
		}
	}
}
