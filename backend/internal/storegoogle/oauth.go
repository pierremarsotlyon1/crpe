package storegoogle

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pierremarsotlyon1/crpe/backend/internal/storeprovider"
)

var raw64 = base64.RawURLEncoding

func (c *Client) bearer(ctx context.Context) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	now := c.now()
	if c.token != "" && now.Add(time.Minute).Before(c.tokenExpires) {
		return c.token, nil
	}
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT", "kid": c.keyID})
	claims, _ := json.Marshal(map[string]any{"iss": c.email, "scope": androidScope, "aud": oauthEndpoint, "iat": now.Unix(), "exp": now.Add(30 * time.Minute).Unix()})
	unsigned := raw64.EncodeToString(header) + "." + raw64.EncodeToString(claims)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, c.signingKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", storeprovider.ErrUnavailable
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {unsigned + "." + raw64.EncodeToString(signature)}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, oauthEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", storeprovider.ErrUnavailable
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := c.http.Do(req)
	if err != nil {
		return "", storeprovider.ErrUnavailable
	}
	defer func() { _ = res.Body.Close() }()
	var token struct {
		Value string `json:"access_token"`
		Type  string `json:"token_type"`
		TTL   int64  `json:"expires_in"`
	}
	if res.StatusCode != http.StatusOK || decodeBody(res.Body, &token) != nil || token.Value == "" || len(token.Value) > 8192 || !strings.EqualFold(token.Type, "Bearer") || token.TTL < 1 || token.TTL > 3600 {
		return "", storeprovider.ErrUnavailable
	}
	c.token, c.tokenExpires = token.Value, now.Add(time.Duration(token.TTL)*time.Second)
	return c.token, nil
}
