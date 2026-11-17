package storegoogle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pierremarsotlyon1/crpe/backend/internal/storeprovider"
)

type lineItem struct {
	Deferred *struct {
		Product string `json:"productId"`
	} `json:"deferredItemReplacement"`
	Replaces *struct {
		Product  string `json:"productId"`
		BasePlan string `json:"basePlanId"`
		Mode     string `json:"replacementMode"`
	} `json:"itemReplacement"`
	Product string `json:"productId"`
	Expiry  string `json:"expiryTime"`
	Offer   struct {
		BasePlan string `json:"basePlanId"`
	} `json:"offerDetails"`
	Renewal *struct {
		Enabled bool `json:"autoRenewEnabled"`
	} `json:"autoRenewingPlan"`
}
type externalAccount struct {
	ID string `json:"obfuscatedExternalAccountId"`
}
type purchase struct {
	OutOfApp *struct {
		ExpiredAccount externalAccount `json:"expiredExternalAccountIdentifiers"`
	} `json:"outOfAppPurchaseContext"`
	Kind    string          `json:"kind"`
	State   string          `json:"subscriptionState"`
	Ack     string          `json:"acknowledgementState"`
	Linked  string          `json:"linkedPurchaseToken"`
	Test    json.RawMessage `json:"testPurchase"`
	Account externalAccount `json:"externalAccountIdentifiers"`
	Items   []lineItem      `json:"lineItems"`
}

// entitlementItem resolves the two successive periods in a deferred replacement.
// Simultaneous add-ons do not correspond to a single membership tier.
func entitlementItem(p purchase, now time.Time) (lineItem, error) {
	if len(p.Items) == 1 {
		return p.Items[0], nil
	}
	if len(p.Items) != 2 || p.Linked == "" {
		return lineItem{}, storeprovider.ErrInvalid
	}
	a, b := p.Items[0], p.Items[1]
	if a.Product == b.Product && a.Offer.BasePlan == b.Offer.BasePlan {
		return lineItem{}, storeprovider.ErrInvalid
	}
	if a.Deferred != nil || b.Deferred != nil {
		if a.Deferred == nil {
			a, b = b, a
		}
		if b.Deferred != nil || a.Deferred.Product != b.Product || a.Expiry == "" || b.Expiry != "" {
			return lineItem{}, storeprovider.ErrInvalid
		}
		return a, nil
	}
	// Play keeps both line items after replacement. The previous period is over.
	earlier, err := time.Parse(time.RFC3339Nano, a.Expiry)
	if err != nil {
		return lineItem{}, storeprovider.ErrInvalid
	}
	later, err := time.Parse(time.RFC3339Nano, b.Expiry)
	if err != nil {
		return lineItem{}, storeprovider.ErrInvalid
	}
	if earlier.After(later) {
		a, b = b, a
		earlier, later = later, earlier
	}
	if !earlier.Before(later) || earlier.After(now) {
		return lineItem{}, storeprovider.ErrInvalid
	}
	if b.Replaces != nil && (b.Replaces.Product != a.Product || b.Replaces.BasePlan != "" && b.Replaces.BasePlan != a.Offer.BasePlan) {
		return lineItem{}, storeprovider.ErrInvalid
	}
	return b, nil
}

// Verify fetches the current state scoped to the server-configured Android application.
func (c *Client) Verify(ctx context.Context, reference string) (storeprovider.Snapshot, error) {
	p, err := c.lookup(ctx, reference)
	if err != nil {
		return storeprovider.Snapshot{}, err
	}
	return c.snapshot(p, reference)
}
func (c *Client) lookup(ctx context.Context, reference string) (purchase, error) {
	var p purchase
	if c == nil {
		return p, storeprovider.ErrUnavailable
	}
	if !validReference(reference) {
		return p, storeprovider.ErrInvalid
	}
	err := c.publisher(ctx, http.MethodGet, "/purchases/subscriptionsv2/tokens/"+url.PathEscape(reference), nil, &p)
	return p, err
}
func (c *Client) snapshot(p purchase, reference string) (storeprovider.Snapshot, error) {
	var s storeprovider.Snapshot
	if p.Kind != "androidpublisher#subscriptionPurchaseV2" {
		return s, storeprovider.ErrInvalid
	}
	now := c.now().UTC()
	item, err := entitlementItem(p, now)
	if err != nil {
		return s, err
	}
	if !validReference(item.Product) || !validReference(item.Offer.BasePlan) || len(item.Product) > 255 || len(item.Offer.BasePlan) > 255 || p.Linked != "" && (!validReference(p.Linked) || p.Linked == reference) {
		return s, storeprovider.ErrInvalid
	}
	environment := "Production"
	if len(p.Test) > 0 && string(p.Test) != "null" {
		environment = "Sandbox"
	}
	if c.environment != environment {
		return s, storeprovider.ErrInvalid
	}
	var expiry time.Time
	if item.Expiry != "" {
		var err error
		expiry, err = time.Parse(time.RFC3339Nano, item.Expiry)
		if err != nil {
			return s, storeprovider.ErrInvalid
		}
	}
	state := ""
	switch p.State {
	case "SUBSCRIPTION_STATE_ACTIVE", "SUBSCRIPTION_STATE_IN_GRACE_PERIOD", "SUBSCRIPTION_STATE_CANCELED":
		if expiry.IsZero() {
			return s, storeprovider.ErrInvalid
		}
		state = "active"
		if !expiry.After(now) {
			state = "expired"
		}
	case "SUBSCRIPTION_STATE_PENDING":
		state = "pending"
	case "SUBSCRIPTION_STATE_PENDING_PURCHASE_CANCELED", "SUBSCRIPTION_STATE_EXPIRED", "SUBSCRIPTION_STATE_PAUSED", "SUBSCRIPTION_STATE_ON_HOLD":
		state = "expired"
	default:
		return s, storeprovider.ErrInvalid
	}
	if p.Ack != "ACKNOWLEDGEMENT_STATE_PENDING" && p.Ack != "ACKNOWLEDGEMENT_STATE_ACKNOWLEDGED" {
		return s, storeprovider.ErrInvalid
	}
	accountID := p.Account.ID
	if accountID == "" && p.OutOfApp != nil {
		accountID = p.OutOfApp.ExpiredAccount.ID
	}
	if len(accountID) > 64 || strings.TrimSpace(accountID) != accountID {
		return s, storeprovider.ErrInvalid
	}
	s = storeprovider.Snapshot{Reference: reference, OriginalReference: reference, LinkedReference: p.Linked, AccountToken: accountID, ProductID: item.Product, BasePlanID: item.Offer.BasePlan, State: state, Environment: environment, ExpiresAt: expiry, ObservedAt: now, AutoRenew: item.Renewal != nil && item.Renewal.Enabled, NeedsAcknowledgement: state == "active" && p.Ack == "ACKNOWLEDGEMENT_STATE_PENDING"}
	return s, nil
}

// Acknowledge is safe to retry after a lost response. The caller must commit the entitlement first.
func (c *Client) Acknowledge(ctx context.Context, s storeprovider.Snapshot) error {
	if c == nil {
		return storeprovider.ErrUnavailable
	}
	if !s.NeedsAcknowledgement {
		return nil
	}
	p, err := c.lookup(ctx, s.Reference)
	if err != nil {
		return err
	}
	latest, err := c.snapshot(p, s.Reference)
	if err != nil {
		return err
	}
	if s.AccountToken == "" || latest.AccountToken != s.AccountToken || latest.Environment != s.Environment {
		return storeprovider.ErrInvalid
	}
	// The worker has already committed s. A concurrent cancellation, plan change or
	// renewal must be committed by a fresh reconciliation before reporting success.
	if latest.State != s.State || latest.ProductID != s.ProductID ||
		latest.BasePlanID != s.BasePlanID || !latest.ExpiresAt.Equal(s.ExpiresAt) {
		return storeprovider.ErrUnavailable
	}
	if !latest.NeedsAcknowledgement {
		return nil
	}
	body := []byte("{}")
	// Play clears outOfAppPurchaseContext after acknowledgement. Persist the verified
	// obfuscated ID so subsequent renewals and refunds retain the same owner.
	if p.OutOfApp != nil && p.Account.ID == "" && latest.AccountToken != "" {
		body, err = json.Marshal(map[string]map[string]string{"externalAccountIds": {"obfuscatedAccountId": latest.AccountToken}})
		if err != nil {
			return storeprovider.ErrInvalid
		}
	}
	return c.publisher(ctx, http.MethodPost, "/purchases/subscriptions/"+url.PathEscape(latest.ProductID)+"/tokens/"+url.PathEscape(s.Reference)+":acknowledge", body, nil)
}
