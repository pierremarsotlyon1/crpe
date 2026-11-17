// Package storebilling reconciles verified store subscriptions with member rights.
package storebilling

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pierremarsotlyon1/crpe/backend/internal/storeprovider"
)

var (
	ErrInvalid  = errors.New("unknown store product or purchase")
	ErrOwner    = errors.New("purchase is linked to a different account")
	ErrConflict = errors.New("another subscription or purchase is in progress")
	ErrDisabled = errors.New("new store purchases disabled")
)

type Product struct {
	Tier       string `json:"tier"`
	ProductID  string `json:"productId"`
	BasePlanID string `json:"basePlanId,omitempty"`
}
type Mapping struct {
	Product
	Platform string `json:"platform"`
	PlanKey  string `json:"planKey"`
}
type Service struct {
	db          *pgxpool.Pool
	products    []Mapping
	providers   map[string]storeprovider.Provider
	environment map[string]string
	aead        cipher.AEAD
}

func New(db *pgxpool.Pool, get func(string) string, providers map[string]storeprovider.Provider) (*Service, error) {
	s := &Service{db: db, providers: providers, environment: map[string]string{"apple": get("APPLE_STORE_ENVIRONMENT"), "google": get("GOOGLE_PLAY_ENVIRONMENT")}}
	if raw := get("STORE_PRODUCTS_JSON"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &s.products); err != nil {
			return nil, fmt.Errorf("invalid STORE_PRODUCTS_JSON")
		}
	}
	products := map[string]bool{}
	for _, p := range s.products {
		k := p.Platform + "/" + p.ProductID + "/" + p.BasePlanID
		if !Platform(p.Platform) || p.ProductID == "" || len(p.ProductID) > 256 || p.PlanKey == "" || len(p.PlanKey) > 160 || p.Tier != "autonomie" && p.Tier != "suivi" || (p.Platform == "google") != (p.BasePlanID != "") || products[k] {
			return nil, fmt.Errorf("invalid or duplicate store product mapping")
		}
		products[k] = true
	}
	if raw := get("STORE_REFERENCE_KEY"); raw != "" || len(providers) > 0 {
		key, e := base64.StdEncoding.DecodeString(raw)
		if e != nil || len(key) != 32 {
			return nil, fmt.Errorf("STORE_REFERENCE_KEY requires a base64 encoded 32-byte key")
		}
		block, e := aes.NewCipher(key)
		if e != nil {
			return nil, e
		}
		s.aead, e = cipher.NewGCMWithRandomNonce(block)
		if e != nil {
			return nil, e
		}
	}
	for platform := range providers {
		if e := s.environment[platform]; !Platform(platform) || (e != "Production" && e != "Sandbox") {
			return nil, errors.New("invalid store provider environment")
		}
	}
	return s, nil
}
func Platform(p string) bool { return p == "apple" || p == "google" }
func accountToken(p, owner string) string {
	id := strings.ToLower(owner)
	if p == "apple" {
		return id
	}
	digest := sha256.Sum256([]byte(id))
	return hex.EncodeToString(digest[:])
}
func (s *Service) mapping(platform, id, base string) (Mapping, bool) {
	for _, p := range s.products {
		if p.Platform == platform && p.ProductID == id && p.BasePlanID == base {
			return p, true
		}
	}
	return Mapping{}, false
}

type ActiveProvider struct {
	Provider      string    `json:"provider"`
	Tier          string    `json:"tier"`
	PlanKey       string    `json:"planKey"`
	ExpiresAt     time.Time `json:"expiresAt"`
	ManagementURL string    `json:"managementUrl,omitempty"`
}

func Active(ctx context.Context, db *pgxpool.Pool, owner string, supportKeys []string) ([]ActiveProvider, error) {
	rows, e := db.Query(ctx, `SELECT source,CASE WHEN tier='suivi' OR (source='stripe' AND plan_key=ANY($2::text[])) THEN 'suivi' ELSE 'autonomie' END,plan_key,valid_until FROM member_rights WHERE user_id=$1 AND source IN('apple','google','stripe') AND NOT refunded AND valid_until>now() ORDER BY valid_until DESC,source,plan_key`, owner, supportKeys)
	if e != nil {
		return nil, e
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (ActiveProvider, error) {
		var v ActiveProvider
		e := r.Scan(&v.Provider, &v.Tier, &v.PlanKey, &v.ExpiresAt)
		switch v.Provider {
		case "apple":
			v.ManagementURL = "https://apps.apple.com/account/subscriptions"
		case "google":
			v.ManagementURL = "https://play.google.com/store/account/subscriptions"
		}
		return v, e
	})
}

type Catalog struct {
	Enabled          bool      `json:"enabled"`
	Items            []Product `json:"items"`
	AccountToken     string    `json:"accountToken"`
	ActiveProvider   string    `json:"activeProvider,omitempty"`
	ActiveProductID  string    `json:"activeProductId,omitempty"`
	ActiveBasePlanID string    `json:"activeBasePlanId,omitempty"`
	PrivacyURL       string    `json:"privacyUrl,omitempty"`
	TermsURL         string    `json:"termsUrl,omitempty"`
}

func (s *Service) Catalog(ctx context.Context, owner, platform, web string, sales bool, supportKeys []string) (Catalog, error) {
	c := Catalog{Items: []Product{}, AccountToken: accountToken(platform, owner), PrivacyURL: web + "/informations/confidentialite", TermsURL: web + "/informations/conditions"}
	if !Platform(platform) {
		return c, ErrInvalid
	}
	active, e := Active(ctx, s.db, owner, supportKeys)
	if e != nil {
		return c, e
	}
	// Any other channel prevents a second payment sheet, even for legacy overlapping subscriptions.
	for _, v := range active {
		c.ActiveProvider = v.Provider
		if v.Provider != platform {
			break
		}
	}
	if c.ActiveProvider == platform {
		e = s.db.QueryRow(ctx, `SELECT product_id,base_plan_id FROM store_memberships WHERE user_id=$1 AND platform=$2 AND state='active' AND NOT superseded AND expires_at>now() ORDER BY expires_at DESC LIMIT 1`, owner, platform).Scan(&c.ActiveProductID, &c.ActiveBasePlanID)
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return c, e
		}
	}
	if sales && s.providers[platform] != nil && (c.ActiveProvider == "" || c.ActiveProvider == platform) {
		for _, p := range s.products {
			if platform == p.Platform {
				c.Items = append(c.Items, p.Product)
			}
		}
		c.Enabled = len(c.Items) > 0
	}
	return c, nil
}
