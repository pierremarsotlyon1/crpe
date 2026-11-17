package storebilling

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pierremarsotlyon1/crpe/backend/internal/storeprovider"
	"github.com/pierremarsotlyon1/crpe/backend/internal/worker"
)

type Receipt struct {
	Status    string     `json:"status"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

func digest(s string) []byte { h := sha256.Sum256([]byte(s)); return h[:] }
func lockRef(ctx context.Context, tx pgx.Tx, platform, ref string) error {
	_, e := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,1313))`, fmt.Sprintf("%s/%x", platform, digest(ref)))
	return e
}
func validRef(platform, ref string) bool {
	if len(ref) == 0 || len(ref) > 4096 || strings.TrimSpace(ref) != ref {
		return false
	}
	if platform == "apple" {
		if len(ref) > 30 {
			return false
		}
		for _, c := range ref {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return Platform(platform)
}
func (s *Service) verify(ctx context.Context, tx pgx.Tx, platform, ref string) (storeprovider.Snapshot, error) {
	var v storeprovider.Snapshot
	if !validRef(platform, ref) {
		return v, ErrInvalid
	}
	p := s.providers[platform]
	if p == nil {
		return v, storeprovider.ErrUnavailable
	}
	if err := lockRef(ctx, tx, platform, ref); err != nil {
		return v, err
	}
	v, err := p.Verify(ctx, ref)
	if err != nil {
		return v, err
	}
	// Individual Apple transaction IDs can alias the same subscription. Verify again under
	// its canonical lock so a slow restore cannot overwrite a more recent notification.
	if platform == "apple" && v.OriginalReference != ref {
		canonical := v.OriginalReference
		if !validRef(platform, canonical) {
			return v, ErrInvalid
		}
		if err = lockRef(ctx, tx, platform, canonical); err != nil {
			return v, err
		}
		v, err = p.Verify(ctx, canonical)
		if err != nil {
			return v, err
		}
		if v.OriginalReference != canonical {
			return v, ErrInvalid
		}
	}
	if !validRef(platform, v.OriginalReference) || !validRef(platform, v.Reference) || v.Environment != s.environment[platform] || v.ObservedAt.IsZero() {
		return v, ErrInvalid
	}
	switch v.State {
	case "active":
		if v.ExpiresAt.IsZero() {
			return v, ErrInvalid
		}
		var expired bool
		if err = tx.QueryRow(ctx, "SELECT clock_timestamp()>=$1::timestamptz", v.ExpiresAt).Scan(&expired); err != nil {
			return v, err
		}
		if expired {
			v.State = "expired"
		}
	case "pending", "expired", "revoked":
	default:
		return v, ErrInvalid
	}
	return v, nil
}
func (s *Service) Purchase(ctx context.Context, owner, platform, product, ref string) (Receipt, error) {
	var out Receipt
	if product == "" || len(product) > 256 || !validRef(platform, ref) {
		return out, ErrInvalid
	}
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		v, e := s.verify(ctx, tx, platform, ref)
		if e != nil {
			return e
		}
		// product is a client hint. A restored transaction can identify the previous product of an upgrade.
		if v.AccountToken == "" || !strings.EqualFold(v.AccountToken, accountToken(platform, owner)) {
			return ErrOwner
		}
		out, e = s.apply(ctx, tx, owner, platform, v)
		return e
	})
	return out, err
}
func (s *Service) Notify(ctx context.Context, platform string, headers http.Header, body []byte) error {
	if !Platform(platform) {
		return ErrInvalid
	}
	p := s.providers[platform]
	if p == nil {
		return storeprovider.ErrUnavailable
	}
	n, e := p.Notification(ctx, headers, body)
	if e != nil {
		return e
	}
	if n.EventID == "" || len(n.EventID) > 4096 {
		return ErrInvalid
	}
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		tag, e := tx.Exec(ctx, `INSERT INTO store_events(platform,environment,event_hash) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, platform, s.environment[platform], digest(n.EventID))
		if e != nil || tag.RowsAffected() == 0 || n.Reference == "" {
			return e
		}
		v, e := s.verify(ctx, tx, platform, n.Reference)
		if e != nil {
			return e
		}
		var owner string
		if v.AccountToken == "" {
			// Only reduce a previously verified entitlement. Possession of a reference
			// without an account identifier cannot establish ownership or grant access.
			if v.State != "revoked" && v.State != "expired" {
				return nil
			}
			var known *string
			e = tx.QueryRow(ctx, `SELECT user_id::text FROM store_memberships WHERE platform=$1 AND environment=$2 AND original_hash=$3`, platform, v.Environment, digest(v.OriginalReference)).Scan(&known)
			if errors.Is(e, pgx.ErrNoRows) || e == nil && known == nil {
				return nil
			}
			if e != nil {
				return e
			}
			owner = *known
		} else if platform == "apple" {
			e = tx.QueryRow(ctx, `SELECT id::text FROM users WHERE id::text=$1`, strings.ToLower(v.AccountToken)).Scan(&owner)
		} else {
			e = tx.QueryRow(ctx, `SELECT id::text FROM users WHERE encode(sha256(convert_to(id::text,'UTF8')),'hex')=$1`, strings.ToLower(v.AccountToken)).Scan(&owner)
		}
		if errors.Is(e, pgx.ErrNoRows) {
			return nil
		}
		if e != nil {
			return e
		}
		e = pgx.BeginFunc(ctx, tx, func(sp pgx.Tx) error { _, err := s.apply(ctx, sp, owner, platform, v); return err })
		if errors.Is(e, ErrOwner) {
			return nil
		}
		return e
	})
}
func (s *Service) apply(ctx context.Context, tx pgx.Tx, owner, platform string, v storeprovider.Snapshot) (Receipt, error) {
	var result Receipt
	mapping, ok := s.mapping(platform, v.ProductID, v.BasePlanID)
	if !ok {
		// Retired products can still expire or be refunded after the public catalogue changes.
		e := tx.QueryRow(ctx, `SELECT plan_key,tier FROM store_memberships WHERE platform=$1 AND environment=$2 AND original_hash=$3 AND user_id=$4 AND (product_id=$5 AND base_plan_id=$6 OR $7 IN ('expired','revoked'))`, platform, v.Environment, digest(v.OriginalReference), owner, v.ProductID, v.BasePlanID, v.State).Scan(&mapping.PlanKey, &mapping.Tier)
		if errors.Is(e, pgx.ErrNoRows) {
			return result, ErrInvalid
		}
		if e != nil {
			return result, e
		}
	}
	if v.State == "active" && v.LinkedReference != "" && v.LinkedReference != v.OriginalReference {
		if e := lockRef(ctx, tx, platform, v.LinkedReference); e != nil {
			return result, e
		}
	}
	var different bool
	if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT FROM store_replacements r JOIN store_memberships m ON m.id=r.successor WHERE r.platform=$1 AND r.environment=$2 AND r.replaced_hash=$3 AND m.user_id IS DISTINCT FROM $4::uuid)`, platform, v.Environment, digest(v.OriginalReference), owner).Scan(&different); e != nil {
		return result, e
	}
	if different {
		return result, ErrOwner
	}
	var locked string
	if e := tx.QueryRow(ctx, "SELECT id::text FROM users WHERE id=$1 FOR UPDATE", owner).Scan(&locked); e != nil {
		return result, e
	}
	original := digest(v.OriginalReference)
	aad := []byte(platform + "/" + v.Environment)
	encrypted := s.aead.Seal(nil, nil, []byte(v.OriginalReference), aad)
	var expires *time.Time
	if !v.ExpiresAt.IsZero() {
		expires = &v.ExpiresAt
	}
	var id string
	// A null owner is the tombstone of a deleted account. It can never be rebound.
	e := tx.QueryRow(ctx, `INSERT INTO store_memberships AS m(platform,environment,original_hash,lookup_cipher,user_id,plan_key,product_id,base_plan_id,tier,state,expires_at,observed_at,auto_renew,acknowledged,superseded)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,clock_timestamp(),$12,$13,EXISTS(SELECT FROM store_replacements WHERE platform=$1 AND environment=$2 AND replaced_hash=$3))
 ON CONFLICT(platform,environment,original_hash) DO UPDATE SET lookup_cipher=excluded.lookup_cipher,plan_key=excluded.plan_key,product_id=excluded.product_id,base_plan_id=excluded.base_plan_id,tier=excluded.tier,state=excluded.state,expires_at=excluded.expires_at,observed_at=excluded.observed_at,auto_renew=excluded.auto_renew,acknowledged=m.acknowledged OR excluded.acknowledged
 WHERE m.user_id=excluded.user_id
 RETURNING id::text`, platform, v.Environment, original, encrypted, owner, mapping.PlanKey, v.ProductID, v.BasePlanID, mapping.Tier, v.State, expires, v.AutoRenew, !v.NeedsAcknowledgement && v.State == "active").Scan(&id)
	if errors.Is(e, pgx.ErrNoRows) {
		return result, ErrOwner
	}
	if e != nil {
		return result, e
	}
	if v.State == "active" && v.LinkedReference != "" && v.LinkedReference != v.OriginalReference {
		hash := digest(v.LinkedReference)
		var existingOwner *string
		e = tx.QueryRow(ctx, `SELECT user_id::text FROM store_memberships WHERE platform=$1 AND environment=$2 AND original_hash=$3`, platform, v.Environment, hash).Scan(&existingOwner)
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return result, e
		}
		if errors.Is(e, pgx.ErrNoRows) {
			parent, verifyErr := s.providers[platform].Verify(ctx, v.LinkedReference)
			// Unqueryable expired Play tokens may predate this ledger. The signed-in purchase
			// is still verified and its linked token is permanently marked as superseded.
			if verifyErr != nil && !errors.Is(verifyErr, storeprovider.ErrInvalid) {
				return result, verifyErr
			}
			if verifyErr == nil && (parent.Environment != v.Environment || !strings.EqualFold(parent.AccountToken, accountToken(platform, owner))) {
				return result, ErrOwner
			}
		}
		if e == nil && (existingOwner == nil || *existingOwner != owner) {
			return result, ErrOwner
		}
		if _, e = tx.Exec(ctx, `INSERT INTO store_replacements VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, platform, v.Environment, hash, id); e != nil {
			return result, e
		}
		if _, e = tx.Exec(ctx, `UPDATE store_memberships SET superseded=true WHERE platform=$1 AND environment=$2 AND original_hash=$3`, platform, v.Environment, hash); e != nil {
			return result, e
		}
	}
	var acknowledged bool
	e = tx.QueryRow(ctx, `SELECT CASE WHEN superseded THEN 'expired' ELSE state END,expires_at,acknowledged FROM store_memberships WHERE id=$1`, id).Scan(&result.Status, &result.ExpiresAt, &acknowledged)
	if e != nil {
		return result, e
	}
	if result.Status == "active" && v.NeedsAcknowledgement && !acknowledged {
		_, e = tx.Exec(ctx, `INSERT INTO jobs(kind,payload,deduplication_key,max_attempts) VALUES('ack_store_purchase',jsonb_build_object('id',$1::text),'store-ack/'||$1,50)
   ON CONFLICT(deduplication_key) DO UPDATE SET status='pending',attempts=0,available_at=now(),completed_at=NULL,last_error=NULL WHERE jobs.status IN('failed','completed')`, id)
	}
	return result, e
}

// AcknowledgeHandler runs only after the verified purchase and entitlement commit. Reference
// ciphertext stays in the ledger; delivery payloads, logs and public exports only use local IDs.
func (s *Service) AcknowledgeHandler() worker.Handler {
	return func(ctx context.Context, m worker.Message) error {
		var in struct{ ID string }
		if json.Unmarshal(m.Payload, &in) != nil || len(in.ID) != 36 {
			return ErrInvalid
		}
		var platform, env, owner string
		var ciphertext []byte
		e := s.db.QueryRow(ctx, `SELECT platform,environment,user_id::text,lookup_cipher FROM store_memberships WHERE id=$1 AND NOT acknowledged AND NOT superseded AND user_id IS NOT NULL`, in.ID).Scan(&platform, &env, &owner, &ciphertext)
		if errors.Is(e, pgx.ErrNoRows) {
			return nil
		}
		if e != nil {
			return e
		}
		p := s.providers[platform]
		if p == nil || s.aead == nil || s.environment[platform] != env {
			return storeprovider.ErrUnavailable
		}
		raw, e := s.aead.Open(nil, nil, ciphertext, []byte(platform+"/"+env))
		if e != nil {
			return errors.New("cannot decrypt store reference")
		}
		var latest storeprovider.Snapshot
		var receipt Receipt
		e = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
			var err error
			latest, err = s.verify(ctx, tx, platform, string(raw))
			if err != nil {
				return err
			}
			if latest.OriginalReference != string(raw) {
				return ErrOwner
			}
			if !strings.EqualFold(latest.AccountToken, accountToken(platform, owner)) && !(latest.AccountToken == "" && (latest.State == "expired" || latest.State == "revoked")) {
				return ErrOwner
			}
			receipt, err = s.apply(ctx, tx, owner, platform, latest)
			return err
		})
		if errors.Is(e, pgx.ErrNoRows) {
			return nil
		}
		if e != nil {
			return e
		}
		if receipt.Status != "active" || !latest.NeedsAcknowledgement {
			return nil
		}
		// Commit the current plan and expiry before acknowledgement. Serialize concurrent
		// worker retries; no provider acknowledgement can precede the durable benefit.
		return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
			var currentProduct, base string
			var acknowledged bool
			err := tx.QueryRow(ctx, `SELECT product_id,base_plan_id,acknowledged FROM store_memberships WHERE id=$1 AND user_id=$2 AND state='active' AND NOT superseded AND expires_at>now() FOR UPDATE`, in.ID, owner).Scan(&currentProduct, &base, &acknowledged)
			if errors.Is(err, pgx.ErrNoRows) || err == nil && acknowledged {
				return nil
			}
			if err != nil {
				return err
			}
			if currentProduct != latest.ProductID || base != latest.BasePlanID {
				return errors.New("store product changed while acknowledging")
			}
			if err = p.Acknowledge(ctx, latest); err != nil {
				return err
			}
			_, err = tx.Exec(ctx, "UPDATE store_memberships SET acknowledged=true WHERE id=$1", in.ID)
			return err
		})
	}
}
