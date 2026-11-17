package billing

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/jackc/pgx/v5"
)

// CloseAccount cancels recurring billing before the caller deletes local data.
// Stripe customer deletion also prevents previously issued checkout links from
// creating a new subscription. This remains available when sales are disabled.
func (s *Service) CloseAccount(ctx context.Context, userID string, tx pgx.Tx) error {
	var customer string
	err := tx.QueryRow(ctx, `SELECT stripe_customer_id FROM billing_customers WHERE user_id=$1`, userID).Scan(&customer)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if s.key == "" {
		return ErrUnavailable
	}
	var result struct {
		ID      string `json:"id"`
		Deleted bool   `json:"deleted"`
	}
	path := "/v1/customers/" + url.PathEscape(customer)
	// Deleted customers can still be retrieved: retries after a local database
	// failure do not require another irreversible provider action.
	if err = s.request(ctx, http.MethodGet, path, nil, "", &result); err != nil {
		return err
	}
	if result.ID != customer {
		return fmt.Errorf("invalid customer response")
	}
	if result.Deleted {
		return nil
	}
	if err = s.request(ctx, http.MethodDelete, path, nil, "close-account-"+userID, &result); err != nil {
		return err
	}
	if !result.Deleted || result.ID != customer {
		return fmt.Errorf("customer closure was not confirmed")
	}
	return nil
}
