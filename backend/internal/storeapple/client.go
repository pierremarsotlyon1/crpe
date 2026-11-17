// Package storeapple verifies subscriptions with the App Store Server API.
package storeapple

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"embed"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pierremarsotlyon1/crpe/backend/internal/storeprovider"
)

// These are public Apple trust anchors, not application secrets.
//
//go:embed certificates/*.cer
var certificates embed.FS

type Client struct {
	client                        *http.Client
	endpoint, bundle, environment string
	issuer, keyID                 string
	appID                         int64
	key                           *ecdsa.PrivateKey
	roots                         *x509.CertPool
	now                           func() time.Time
}

var _ storeprovider.Provider = (*Client)(nil)

func FromEnv(lookup func(string) string) (*Client, error) {
	values := []string{"APPLE_STORE_BUNDLE_ID", "APPLE_STORE_APP_ID", "APPLE_STORE_KEY_ID", "APPLE_STORE_ISSUER_ID", "APPLE_STORE_PRIVATE_KEY", "APPLE_STORE_ENVIRONMENT"}
	configured := false
	for _, name := range values {
		configured = configured || lookup(name) != ""
	}
	if !configured {
		return nil, nil
	}
	for _, name := range values {
		if strings.TrimSpace(lookup(name)) == "" {
			return nil, errors.New("complete Apple Store configuration required")
		}
	}
	c := &Client{bundle: lookup(values[0]), keyID: lookup(values[2]), issuer: lookup(values[3]), environment: lookup(values[5]), now: time.Now}
	if len(c.bundle) > 255 || strings.ContainsAny(c.bundle, " /\r\n") || len(c.keyID) > 128 || !validUUID(c.issuer) {
		return nil, errors.New("invalid Apple Store identifiers")
	}
	var err error
	c.appID, err = strconv.ParseInt(lookup(values[1]), 10, 64)
	if err != nil || c.appID <= 0 {
		return nil, errors.New("invalid Apple Store app ID")
	}
	switch c.environment {
	case "Production":
		c.endpoint = "https://api.storekit.apple.com"
	case "Sandbox":
		c.endpoint = "https://api.storekit-sandbox.apple.com"
	default:
		return nil, errors.New("invalid Apple Store environment: use Sandbox or Production")
	}
	block, rest := pem.Decode([]byte(lookup(values[4])))
	if block == nil || block.Type != "PRIVATE KEY" || len(strings.TrimSpace(string(rest))) != 0 {
		return nil, errors.New("invalid Apple Store private key")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("invalid Apple Store private key")
	}
	c.key, _ = key.(*ecdsa.PrivateKey)
	if c.key == nil || c.key.Curve != elliptic.P256() {
		return nil, errors.New("an ES256 private key is required for Apple Store")
	}
	c.roots = x509.NewCertPool()
	for _, name := range []string{"AppleRootCA-G2.cer", "AppleRootCA-G3.cer"} {
		der, err := certificates.ReadFile("certificates/" + name)
		if err != nil {
			return nil, errors.New("missing Apple trust anchor")
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, errors.New("invalid Apple trust anchor")
		}
		c.roots.AddCert(cert)
	}
	c.client = &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return c, nil
}

func (c *Client) request(ctx context.Context, path string, out any) error {
	credential, err := c.credential()
	if err != nil {
		return storeprovider.ErrUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+path, nil)
	if err != nil {
		return storeprovider.ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+credential)
	resp, err := c.client.Do(req)
	if err != nil {
		return storeprovider.ErrUnavailable
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusBadRequest {
		return storeprovider.ErrInvalid
	}
	if resp.StatusCode != http.StatusOK {
		return storeprovider.ErrUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 || json.Unmarshal(body, out) != nil {
		return storeprovider.ErrUnavailable
	}
	return nil
}

type transaction struct {
	TransactionID         string `json:"transactionId"`
	OriginalTransactionID string `json:"originalTransactionId"`
	AccountToken          string `json:"appAccountToken"`
	BundleID              string `json:"bundleId"`
	ProductID             string `json:"productId"`
	Environment           string `json:"environment"`
	Type                  string `json:"type"`
	Ownership             string `json:"inAppOwnershipType"`
	ExpiresDate           int64  `json:"expiresDate"`
	SignedDate            int64  `json:"signedDate"`
	RevocationDate        int64  `json:"revocationDate"`
	IsUpgraded            bool   `json:"isUpgraded"`
}

func (c *Client) decodeTransaction(signed string) (transaction, error) {
	var t transaction
	if err := c.verifySigned(signed, &t); err != nil {
		return t, err
	}
	if t.BundleID != c.bundle || t.Environment != c.environment || t.Type != "Auto-Renewable Subscription" || t.Ownership != "PURCHASED" || !validReference(t.TransactionID) || !validReference(t.OriginalTransactionID) || !validUUID(t.AccountToken) || t.ProductID == "" || t.ExpiresDate <= 0 {
		return t, storeprovider.ErrInvalid
	}
	t.AccountToken = strings.ToLower(t.AccountToken)
	return t, nil
}

// Verify refreshes the entire original subscription so replaying an older signed
// transaction cannot resurrect a refunded or superseded subscription.
func (c *Client) Verify(ctx context.Context, reference string) (storeprovider.Snapshot, error) {
	var out storeprovider.Snapshot
	if c == nil {
		return out, storeprovider.ErrUnavailable
	}
	if !validReference(reference) {
		return out, storeprovider.ErrInvalid
	}
	var response struct {
		SignedTransactionInfo string `json:"signedTransactionInfo"`
	}
	if err := c.request(ctx, "/inApps/v1/transactions/"+reference, &response); err != nil {
		return out, err
	}
	original, err := c.decodeTransaction(response.SignedTransactionInfo)
	if err != nil {
		return out, err
	}
	if original.TransactionID != reference && original.OriginalTransactionID != reference {
		return out, storeprovider.ErrInvalid
	}
	var current struct {
		AppID       int64  `json:"appAppleId"`
		BundleID    string `json:"bundleId"`
		Environment string `json:"environment"`
		Data        []struct {
			LastTransactions []struct {
				Status                int    `json:"status"`
				OriginalTransactionID string `json:"originalTransactionId"`
				Transaction           string `json:"signedTransactionInfo"`
				Renewal               string `json:"signedRenewalInfo"`
			} `json:"lastTransactions"`
		} `json:"data"`
	}
	if err := c.request(ctx, "/inApps/v1/subscriptions/"+original.OriginalTransactionID, &current); err != nil {
		return out, err
	}
	if !c.applicationMatches(current.BundleID, current.Environment, current.AppID) {
		return out, storeprovider.ErrInvalid
	}
	found := false
	for _, group := range current.Data {
		for _, item := range group.LastTransactions {
			if item.OriginalTransactionID != original.OriginalTransactionID {
				continue
			}
			if found {
				return out, storeprovider.ErrInvalid
			}
			found = true
			latest, err := c.decodeTransaction(item.Transaction)
			if err != nil || latest.OriginalTransactionID != original.OriginalTransactionID || latest.AccountToken != original.AccountToken {
				return out, storeprovider.ErrInvalid
			}
			var renewal struct {
				OriginalTransactionID string `json:"originalTransactionId"`
				Environment           string `json:"environment"`
				ProductID             string `json:"productId"`
				AutoRenewStatus       int    `json:"autoRenewStatus"`
				GraceExpires          int64  `json:"gracePeriodExpiresDate"`
			}
			if err := c.verifySigned(item.Renewal, &renewal); err != nil || renewal.OriginalTransactionID != latest.OriginalTransactionID || renewal.Environment != c.environment || renewal.ProductID != latest.ProductID {
				return out, storeprovider.ErrInvalid
			}
			out = storeprovider.Snapshot{Reference: latest.TransactionID, OriginalReference: latest.OriginalTransactionID, AccountToken: latest.AccountToken, ProductID: latest.ProductID, State: "expired", Environment: c.environment, ExpiresAt: time.UnixMilli(latest.ExpiresDate), ObservedAt: c.now().UTC(), AutoRenew: renewal.AutoRenewStatus == 1}
			switch item.Status {
			case 1: // Active, including a cancellation effective at period end.
				out.State = "active"
			case 2, 3: // Expired or billing retry without grace.
			case 4:
				out.State = "active"
				out.ExpiresAt = time.UnixMilli(renewal.GraceExpires)
			case 5:
				out.State = "revoked"
			default:
				return out, storeprovider.ErrInvalid
			}
			if latest.RevocationDate != 0 || latest.IsUpgraded {
				out.State = "revoked"
			}
			if out.State == "active" && !out.ExpiresAt.After(c.now()) {
				out.State = "expired"
			}
		}
	}
	if !found {
		return out, storeprovider.ErrInvalid
	}
	return out, nil
}

// Acknowledge is a no-op: StoreKit finishes on-device after the membership commit.
func (c *Client) Acknowledge(context.Context, storeprovider.Snapshot) error {
	if c == nil {
		return storeprovider.ErrUnavailable
	}
	return nil
}

func (c *Client) Notification(_ context.Context, _ http.Header, body []byte) (storeprovider.Notification, error) {
	var out storeprovider.Notification
	if c == nil {
		return out, storeprovider.ErrUnavailable
	}
	var wrapper struct {
		SignedPayload string `json:"signedPayload"`
	}
	if len(body) > 512<<10 || json.Unmarshal(body, &wrapper) != nil {
		return out, storeprovider.ErrInvalid
	}
	var notice struct {
		ID      string `json:"notificationUUID"`
		Type    string `json:"notificationType"`
		Subtype string `json:"subtype"`
		Version string `json:"version"`
		Data    *struct {
			AppID       int64  `json:"appAppleId"`
			BundleID    string `json:"bundleId"`
			Environment string `json:"environment"`
			Transaction string `json:"signedTransactionInfo"`
		} `json:"data"`
		Summary *struct {
			AppID       int64  `json:"appAppleId"`
			BundleID    string `json:"bundleId"`
			Environment string `json:"environment"`
		} `json:"summary"`
	}
	if err := c.verifySigned(wrapper.SignedPayload, &notice); err != nil || !validUUID(notice.ID) || notice.Version != "2.0" {
		return out, storeprovider.ErrInvalid
	}
	out.EventID = notice.ID
	// Apple sends this aggregate after the individual extension notifications.
	// It contains no transaction to reconcile and must not grant individual access.
	if notice.Summary != nil {
		s := notice.Summary
		if notice.Data != nil || notice.Type != "RENEWAL_EXTENSION" || notice.Subtype != "SUMMARY" || !c.applicationMatches(s.BundleID, s.Environment, s.AppID) {
			return storeprovider.Notification{}, storeprovider.ErrInvalid
		}
		return out, nil
	}
	if notice.Data == nil || !c.applicationMatches(notice.Data.BundleID, notice.Data.Environment, notice.Data.AppID) {
		return storeprovider.Notification{}, storeprovider.ErrInvalid
	}
	if notice.Type == "TEST" {
		return out, nil
	}
	t, err := c.decodeTransaction(notice.Data.Transaction)
	if err != nil {
		return out, err
	}
	out.Reference = t.OriginalTransactionID
	return out, nil
}

func (c *Client) applicationMatches(bundle, environment string, appID int64) bool {
	return bundle == c.bundle && environment == c.environment &&
		(appID == c.appID || c.environment == "Sandbox" && appID == 0)
}

func validReference(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func validUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range strings.ToLower(s) {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !(c >= '0' && c <= '9') && !(c >= 'a' && c <= 'f') {
			return false
		}
	}
	return s != "00000000-0000-0000-0000-000000000000"
}
