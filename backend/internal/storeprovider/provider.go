// Package storeprovider defines verified store observations for the membership ledger.
package storeprovider

import (
	"context"
	"errors"
	"net/http"
	"time"
)

var ErrInvalid = errors.New("invalid store purchase")
var ErrUnavailable = errors.New("store provider unavailable")

// Snapshot comes from a provider API, never from client-supplied entitlement fields.
// The caller must check the account token and configured product/base-plan allowlist.
type Snapshot struct {
	Reference            string
	OriginalReference    string
	LinkedReference      string
	AccountToken         string
	ProductID            string
	BasePlanID           string
	State                string // active, pending, expired or revoked
	Environment          string // Sandbox or Production
	ExpiresAt            time.Time
	ObservedAt           time.Time
	AutoRenew            bool
	NeedsAcknowledgement bool
}

// Notification is authenticated but conveys no entitlement. Re-fetch Reference.
// An empty Reference is an authenticated test, aggregate or irrelevant event.
type Notification struct {
	Reference string
	EventID   string
}

type Provider interface {
	Verify(context.Context, string) (Snapshot, error)
	Acknowledge(context.Context, Snapshot) error
	Notification(context.Context, http.Header, []byte) (Notification, error)
}
