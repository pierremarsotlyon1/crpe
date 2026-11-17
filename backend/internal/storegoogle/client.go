// Package storegoogle verifies Google Play subscriptions and authenticated Pub/Sub notifications.
package storegoogle

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/pierremarsotlyon1/crpe/backend/internal/storeprovider"
)

const (
	oauthEndpoint   = "https://oauth2.googleapis.com/token"
	publisherOrigin = "https://androidpublisher.googleapis.com/androidpublisher/v3/applications/"
	jwksEndpoint    = "https://www.googleapis.com/oauth2/v3/certs"
	androidScope    = "https://www.googleapis.com/auth/androidpublisher"
)

var packagePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z][A-Za-z0-9_]*)+$`)
var subscriptionPattern = regexp.MustCompile(`^projects/[a-zA-Z0-9.-]+/subscriptions/[a-zA-Z][a-zA-Z0-9._~%+-]*$`)
var emailPattern = regexp.MustCompile(`^[a-zA-Z0-9._+-]+@[a-zA-Z0-9.-]+\.gserviceaccount\.com$`)
var _ storeprovider.Provider = (*Client)(nil)

type Client struct {
	packageName, environment, audience, pushEmail, subscription string
	email, keyID                                                string
	signingKey                                                  *rsa.PrivateKey
	http                                                        *http.Client
	now                                                         func() time.Time
	tokenMu                                                     sync.Mutex
	token                                                       string
	tokenExpires                                                time.Time
	keysMu                                                      sync.Mutex
	keys                                                        map[string]*rsa.PublicKey
	keysExpire, keysUpdated                                     time.Time
}

// FromEnv returns nil when Google Play integration has not been configured.
func FromEnv(get func(string) string) (*Client, error) {
	c := &Client{packageName: get("GOOGLE_PLAY_PACKAGE_NAME"), environment: get("GOOGLE_PLAY_ENVIRONMENT"), audience: get("GOOGLE_PLAY_PUSH_AUDIENCE"), pushEmail: get("GOOGLE_PLAY_PUSH_EMAIL"), subscription: get("GOOGLE_PLAY_PUSH_SUBSCRIPTION"), now: time.Now}
	raw := get("GOOGLE_PLAY_SERVICE_ACCOUNT_JSON")
	if c.packageName == "" && raw == "" && c.environment == "" && c.audience == "" && c.pushEmail == "" && c.subscription == "" {
		return nil, nil
	}
	if !packagePattern.MatchString(c.packageName) || len(c.packageName) > 255 {
		return nil, errors.New("invalid Google Play package name")
	}
	if c.environment != "Production" && c.environment != "Sandbox" {
		return nil, errors.New("invalid Google Play environment: expected Production or Sandbox")
	}
	if len(c.audience) > 2048 || c.audience == "" || !emailPattern.MatchString(c.pushEmail) || !subscriptionPattern.MatchString(c.subscription) {
		return nil, errors.New("invalid Google Play Pub/Sub authentication configuration")
	}
	var account struct {
		Type  string
		Email string `json:"client_email"`
		KeyID string `json:"private_key_id"`
		Key   string `json:"private_key"`
	}
	if len(raw) > 65536 || json.Unmarshal([]byte(raw), &account) != nil || account.Type != "service_account" || !emailPattern.MatchString(account.Email) || account.KeyID == "" {
		return nil, errors.New("invalid Google Play service account")
	}
	block, rest := pem.Decode([]byte(account.Key))
	if block == nil || block.Type != "PRIVATE KEY" || len(strings.TrimSpace(string(rest))) != 0 {
		return nil, errors.New("invalid Google Play private key")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("invalid Google Play private key")
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok || key.N.BitLen() < 2048 || key.Validate() != nil {
		return nil, errors.New("invalid Google Play RSA key")
	}
	c.email, c.keyID, c.signingKey = account.Email, account.KeyID, key
	c.http = &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return c, nil
}

func validReference(s string) bool {
	if s == "" || len(s) > 4096 || s == "." || s == ".." {
		return false
	}
	for _, r := range s {
		if r < 33 || r > 126 || strings.ContainsRune("/\\?#%", r) {
			return false
		}
	}
	return true
}

func decodeBody(r io.Reader, dest any) error {
	const max = 1 << 20
	data, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil || len(data) > max || json.Unmarshal(data, dest) != nil {
		return storeprovider.ErrUnavailable
	}
	return nil
}
func (c *Client) publisher(ctx context.Context, method, path string, payload []byte, out any) error {
	token, err := c.bearer(ctx)
	if err != nil {
		return err
	}
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, publisherOrigin+url.PathEscape(c.packageName)+path, body)
	if err != nil {
		return storeprovider.ErrInvalid
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return storeprovider.ErrUnavailable
	}
	defer func() { _ = res.Body.Close() }()
	switch {
	case res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusBadRequest || res.StatusCode == http.StatusGone:
		return storeprovider.ErrInvalid
	case res.StatusCode == http.StatusUnauthorized:
		c.tokenMu.Lock()
		if c.token == token {
			c.token = ""
		}
		c.tokenMu.Unlock()
		return storeprovider.ErrUnavailable
	case res.StatusCode < 200 || res.StatusCode >= 300:
		return storeprovider.ErrUnavailable
	case out != nil:
		return decodeBody(res.Body, out)
	default:
		return nil
	}
}
