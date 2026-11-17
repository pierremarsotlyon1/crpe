package storeapple

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"strings"
	"time"

	"github.com/pierremarsotlyon1/crpe/backend/internal/storeprovider"
)

func (c *Client) credential() (string, error) {
	header, err := json.Marshal(map[string]string{"alg": "ES256", "kid": c.keyID, "typ": "JWT"})
	if err != nil {
		return "", err
	}
	now := c.now()
	payload, err := json.Marshal(map[string]any{"iss": c.issuer, "iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(), "aud": "appstoreconnect-v1", "bid": c.bundle})
	if err != nil {
		return "", err
	}
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(unsigned))
	r, s, err := ecdsa.Sign(rand.Reader, c.key, digest[:])
	if err != nil {
		return "", err
	}
	signature := make([]byte, 64)
	r.FillBytes(signature[:32])
	s.FillBytes(signature[32:])
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

// verifySigned validates Apple's certificate profile and ES256 signature. Chain
// validity is evaluated at signedDate, as for Apple's offline verifier. No roots
// supplied in a request are trusted. Notifications only trigger a live API refresh.
func (c *Client) verifySigned(value string, out any) error {
	if len(value) == 0 || len(value) > 256<<10 {
		return storeprovider.ErrInvalid
	}
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return storeprovider.ErrInvalid
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || len(headerBytes) > 32<<10 {
		return storeprovider.ErrInvalid
	}
	var header struct {
		Algorithm string   `json:"alg"`
		Chain     []string `json:"x5c"`
		Critical  []string `json:"crit"`
	}
	if json.Unmarshal(headerBytes, &header) != nil || header.Algorithm != "ES256" || len(header.Chain) != 3 || len(header.Critical) != 0 {
		return storeprovider.ErrInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return storeprovider.ErrInvalid
	}
	var date struct {
		SignedDate int64 `json:"signedDate"`
	}
	if json.Unmarshal(payload, &date) != nil || date.SignedDate <= 0 || date.SignedDate > c.now().Add(5*time.Minute).UnixMilli() {
		return storeprovider.ErrInvalid
	}
	certs := make([]*x509.Certificate, 3)
	for i, encoded := range header.Chain {
		der, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return storeprovider.ErrInvalid
		}
		certs[i], err = x509.ParseCertificate(der)
		if err != nil {
			return storeprovider.ErrInvalid
		}
	}
	if certs[0].IsCA || certs[0].KeyUsage&x509.KeyUsageDigitalSignature == 0 || !hasOID(certs[0], asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 11, 1}) || !hasOID(certs[1], asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 2, 1}) {
		return storeprovider.ErrInvalid
	}
	intermediates := x509.NewCertPool()
	intermediates.AddCert(certs[1])
	chains, err := certs[0].Verify(x509.VerifyOptions{Roots: c.roots, Intermediates: intermediates, CurrentTime: time.UnixMilli(date.SignedDate), KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}})
	if err != nil || len(chains) == 0 || len(chains[0]) != 3 || !chains[0][2].Equal(certs[2]) {
		return storeprovider.ErrInvalid
	}
	public, ok := certs[0].PublicKey.(*ecdsa.PublicKey)
	if !ok || public.Curve != elliptic.P256() {
		return storeprovider.ErrInvalid
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(signature) != 64 {
		return storeprovider.ErrInvalid
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(public, digest[:], new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:])) || json.Unmarshal(payload, out) != nil {
		return storeprovider.ErrInvalid
	}
	return nil
}

func hasOID(c *x509.Certificate, oid asn1.ObjectIdentifier) bool {
	for _, extension := range c.Extensions {
		if extension.Id.Equal(oid) {
			return true
		}
	}
	return false
}
