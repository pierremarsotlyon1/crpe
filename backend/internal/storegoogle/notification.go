package storegoogle

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pierremarsotlyon1/crpe/backend/internal/storeprovider"
)

type jwtHeader struct {
	Alg, Kid string
	Crit     []string
}
type pushClaims struct {
	Iss, Aud, Sub, Email string
	Verified             bool  `json:"email_verified"`
	Issued               int64 `json:"iat"`
	Expiry               int64 `json:"exp"`
	NotBefore            int64 `json:"nbf"`
}

func (c *Client) verifyPushIdentity(ctx context.Context, h http.Header) error {
	values := h.Values("Authorization")
	if len(values) != 1 {
		return storeprovider.ErrInvalid
	}
	token, ok := strings.CutPrefix(values[0], "Bearer ")
	if !ok || len(token) > 16384 {
		return storeprovider.ErrInvalid
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return storeprovider.ErrInvalid
	}
	header, err := raw64.DecodeString(parts[0])
	if err != nil {
		return storeprovider.ErrInvalid
	}
	var meta jwtHeader
	if json.Unmarshal(header, &meta) != nil || meta.Alg != "RS256" || meta.Kid == "" || len(meta.Kid) > 256 || len(meta.Crit) != 0 {
		return storeprovider.ErrInvalid
	}
	data, err := raw64.DecodeString(parts[1])
	if err != nil {
		return storeprovider.ErrInvalid
	}
	var claims pushClaims
	if json.Unmarshal(data, &claims) != nil {
		return storeprovider.ErrInvalid
	}
	now := c.now().Unix()
	if (claims.Iss != "https://accounts.google.com" && claims.Iss != "accounts.google.com") || claims.Aud != c.audience || !claims.Verified || claims.Email != c.pushEmail || claims.Sub == "" || claims.Expiry <= now || claims.Issued <= 0 || claims.Issued > now+60 || claims.NotBefore > now+60 || claims.Expiry <= claims.Issued || claims.Expiry-claims.Issued > 3660 {
		return storeprovider.ErrInvalid
	}
	signature, err := raw64.DecodeString(parts[2])
	if err != nil {
		return storeprovider.ErrInvalid
	}
	key, err := c.verificationKey(ctx, meta.Kid)
	if err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature) != nil {
		return storeprovider.ErrInvalid
	}
	return nil
}

type publicJWK struct{ Kid, Kty, Use, Alg, N, E string }

func (c *Client) verificationKey(ctx context.Context, id string) (*rsa.PublicKey, error) {
	c.keysMu.Lock()
	defer c.keysMu.Unlock()
	now := c.now()
	if now.Before(c.keysExpire) {
		if key := c.keys[id]; key != nil {
			return key, nil
		}
		if now.Sub(c.keysUpdated) < time.Minute {
			return nil, storeprovider.ErrInvalid
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksEndpoint, nil)
	if err != nil {
		return nil, storeprovider.ErrUnavailable
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, storeprovider.ErrUnavailable
	}
	defer func() { _ = res.Body.Close() }()
	var set struct{ Keys []publicJWK }
	if res.StatusCode != http.StatusOK || decodeBody(res.Body, &set) != nil || len(set.Keys) > 64 {
		return nil, storeprovider.ErrUnavailable
	}
	keys := make(map[string]*rsa.PublicKey, len(set.Keys))
	for _, j := range set.Keys {
		if j.Kid == "" || j.Kty != "RSA" || j.Use != "sig" || j.Alg != "RS256" {
			continue
		}
		n, nerr := raw64.DecodeString(j.N)
		e, eerr := raw64.DecodeString(j.E)
		if nerr != nil || eerr != nil || len(n) < 256 || len(n) > 1024 || len(e) == 0 || len(e) > 3 {
			return nil, storeprovider.ErrUnavailable
		}
		exp := int(new(big.Int).SetBytes(e).Int64())
		modulus := new(big.Int).SetBytes(n)
		if exp < 3 || exp%2 == 0 || modulus.BitLen() < 2048 || modulus.Bit(0) != 1 || keys[j.Kid] != nil {
			return nil, storeprovider.ErrUnavailable
		}
		keys[j.Kid] = &rsa.PublicKey{N: modulus, E: exp}
	}
	if len(keys) == 0 {
		return nil, storeprovider.ErrUnavailable
	}
	ttl := time.Minute
	for field := range strings.SplitSeq(res.Header.Get("Cache-Control"), ",") {
		if seconds, ok := strings.CutPrefix(strings.TrimSpace(field), "max-age="); ok {
			n, err := strconv.Atoi(seconds)
			if err == nil && n >= 0 {
				ttl = time.Duration(min(n, 21600)) * time.Second
			}
		}
	}
	c.keys, c.keysUpdated, c.keysExpire = keys, now, now.Add(ttl)
	key := keys[id]
	if key == nil {
		return nil, storeprovider.ErrInvalid
	}
	return key, nil
}

// Notification authenticates Pub/Sub's push identity and exact subscription.
// RTDN event names, timestamps and body content never grant an entitlement.
func (c *Client) Notification(ctx context.Context, h http.Header, body []byte) (storeprovider.Notification, error) {
	var result storeprovider.Notification
	if c == nil {
		return result, storeprovider.ErrUnavailable
	}
	if len(body) > 1<<20 {
		return result, storeprovider.ErrInvalid
	}
	if err := c.verifyPushIdentity(ctx, h); err != nil {
		return result, err
	}
	var envelope struct {
		Subscription string
		Message      struct {
			ID   string `json:"messageId"`
			Data string
		}
	}
	if json.Unmarshal(body, &envelope) != nil || envelope.Subscription != c.subscription || envelope.Message.ID == "" || len(envelope.Message.ID) > 128 {
		return result, storeprovider.ErrInvalid
	}
	data, err := base64.StdEncoding.DecodeString(envelope.Message.Data)
	if err != nil || len(data) > 65536 {
		return result, storeprovider.ErrInvalid
	}
	type notice struct {
		Token       string `json:"purchaseToken"`
		ProductType int    `json:"productType"`
	}
	var event struct {
		Version, PackageName string
		Subscription         *notice   `json:"subscriptionNotification"`
		Test                 *struct{} `json:"testNotification"`
		Voided               *notice   `json:"voidedPurchaseNotification"`
		OneTime              *notice   `json:"oneTimeProductNotification"`
	}
	if json.Unmarshal(data, &event) != nil || event.PackageName != c.packageName || event.Version != "1.0" {
		return result, storeprovider.ErrInvalid
	}
	kinds := 0
	for _, present := range []bool{event.Subscription != nil, event.Test != nil, event.Voided != nil, event.OneTime != nil} {
		if present {
			kinds++
		}
	}
	if kinds != 1 {
		return result, storeprovider.ErrInvalid
	}
	result.EventID = envelope.Message.ID
	if event.Test != nil || event.OneTime != nil || event.Voided != nil && event.Voided.ProductType == 2 {
		return result, nil
	}
	if event.Subscription != nil {
		result.Reference = event.Subscription.Token
	} else if event.Voided != nil && event.Voided.ProductType == 1 {
		result.Reference = event.Voided.Token
	}
	if !validReference(result.Reference) {
		return storeprovider.Notification{}, storeprovider.ErrInvalid
	}
	return result, nil
}
