package storebilling

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgtype"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type StripeChecker interface {
	CheckNativeOrder(context.Context, pgx.Tx, string) error
}
type OrderInput struct {
	Platform                 string `json:"platform"`
	ProductID                string `json:"productId"`
	BasePlanID               string `json:"basePlanId"`
	ClientID                 string `json:"clientId"`
	ExpectedActiveProductID  string `json:"expectedActiveProductId"`
	ExpectedActiveBasePlanID string `json:"expectedActiveBasePlanId"`
}
type Order struct {
	ID        string    `json:"id"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// Reserve serializes native/Stripe entry points on the same member row. A later store
// approval or a purchase made outside our app is still reconciled by Purchase/Notify.
func (s *Service) Reserve(ctx context.Context, owner string, in OrderInput, sales bool, stripe StripeChecker) (Order, error) {
	var result Order
	platform, id, base, clientID := in.Platform, in.ProductID, in.BasePlanID, strings.ToLower(in.ClientID)
	if clientID != "" && !orderUUID(clientID) {
		return result, ErrInvalid
	}
	if !sales || s.providers[platform] == nil {
		return result, ErrDisabled
	}
	if _, ok := s.mapping(platform, id, base); !ok {
		return result, ErrInvalid
	}
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var locked string
		if e := tx.QueryRow(ctx, "SELECT id::text FROM users WHERE id=$1 FOR UPDATE", owner).Scan(&locked); e != nil {
			return e
		}
		var active int
		var priorProduct, priorBase string
		if err := tx.QueryRow(ctx, `SELECT count(*),coalesce(min(product_id),''),coalesce(min(base_plan_id),'') FROM store_memberships WHERE user_id=$1 AND platform=$2 AND state='active' AND expires_at>clock_timestamp() AND NOT superseded`, owner, platform).Scan(&active, &priorProduct, &priorBase); err != nil {
			return err
		}
		if active > 1 || in.ExpectedActiveProductID != priorProduct || in.ExpectedActiveBasePlanID != priorBase || active == 1 && id == priorProduct && base == priorBase {
			return ErrConflict
		}
		// A repeat of the exact client operation returns its original reservation.
		var currentPlatform, currentProduct, currentBase, currentClientID string
		e := tx.QueryRow(ctx, `SELECT id::text,expires_at,platform,product_id,base_plan_id,coalesce(client_id::text,'') FROM store_orders WHERE user_id=$1 AND expires_at>now()`, owner).Scan(&result.ID, &result.ExpiresAt, &currentPlatform, &currentProduct, &currentBase, &currentClientID)
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if e == nil && currentPlatform != "stripe" {
			if clientID != "" && currentPlatform == platform && currentProduct == id && currentBase == base && currentClientID == clientID {
				return nil
			}
			return ErrConflict
		}
		var blocked bool
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT FROM member_rights WHERE user_id=$1 AND NOT refunded AND valid_until>now() AND source<>$2)
    OR EXISTS(SELECT FROM store_orders WHERE user_id=$1 AND platform<>$2 AND expires_at>now() AND (platform<>'stripe' OR NOT $3))
    OR EXISTS(SELECT FROM store_memberships WHERE user_id=$1 AND state='pending' AND NOT superseded)`, owner, platform, stripe != nil).Scan(&blocked); e != nil {
			return e
		}
		if blocked {
			return ErrConflict
		}
		if stripe != nil {
			if e := stripe.CheckNativeOrder(ctx, tx, owner); e != nil {
				return e
			}
		}
		return tx.QueryRow(ctx, `INSERT INTO store_orders(user_id,platform,expires_at,product_id,base_plan_id,client_id) VALUES($1,$2,now()+interval '15 minutes',$3,$4,nullif($5,'')::uuid) ON CONFLICT(user_id) DO UPDATE SET platform=excluded.platform,expires_at=excluded.expires_at,id=excluded.id,product_id=excluded.product_id,base_plan_id=excluded.base_plan_id,client_id=excluded.client_id RETURNING id::text,expires_at`, owner, platform, id, base, clientID).Scan(&result.ID, &result.ExpiresAt)
	})
	return result, err
}
func (s *Service) Cancel(ctx context.Context, owner, platform, id string) error {
	if !Platform(platform) || !orderUUID(id) {
		return ErrInvalid
	}
	_, e := s.db.Exec(ctx, `DELETE FROM store_orders WHERE user_id=$1 AND platform=$2 AND id=$3::uuid`, owner, platform, id)
	return e
}

func orderUUID(value string) bool {
	var id pgtype.UUID
	return len(value) == 36 && id.Scan(value) == nil && id.Valid
}
