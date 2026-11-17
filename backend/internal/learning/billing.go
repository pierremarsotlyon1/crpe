package learning

import (
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pierremarsotlyon1/crpe/backend/internal/billing"
)

func billingError(err error) error {
	switch {
	case errors.Is(err, billing.ErrDisabled):
		return missing()
	case errors.Is(err, billing.ErrUnavailable):
		return unavailable()
	case errors.Is(err, billing.ErrActiveSubscription):
		return problem{409, "Un abonnement est déjà actif ou en cours de confirmation. Consultez la gestion de votre abonnement."}
	case errors.Is(err, billing.ErrRefundConflict):
		return problem{409, "Ce remboursement nécessite une vérification dans Stripe."}
	case errors.Is(err, billing.ErrInvalidPlan):
		return bad("Formule inconnue.")
	case errors.Is(err, billing.ErrSignature):
		return bad("Signature Stripe invalide.")
	default:
		return err
	}
}
func (a *App) sales() error {
	if !a.opts.SalesEnabled {
		return missing()
	}
	if a.opts.Billing == nil {
		return unavailable()
	}
	return nil
}
func (a *App) plans(w http.ResponseWriter, r *http.Request, _ User) error {
	if err := a.sales(); err != nil {
		return err
	}
	if err := a.rate(r.Context(), "plans/"+a.requestIP(r), a.opts.AuthIPHourlyLimit*5); err != nil {
		return err
	}
	items, err := a.opts.Billing.Plans(r.Context())
	if err != nil {
		return billingError(err)
	}
	return jsonOut(w, 200, map[string]any{"items": items})
}
func (a *App) checkout(w http.ResponseWriter, r *http.Request, u User) error {
	if err := a.sales(); err != nil {
		return err
	}
	var in struct {
		PlanKey string `json:"planKey"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	if err := a.rate(r.Context(), "checkout/"+u.ID, 8); err != nil {
		return err
	}
	url, err := a.opts.Billing.Checkout(r.Context(), u.ID, u.Email, in.PlanKey)
	if err != nil {
		return billingError(err)
	}
	return jsonOut(w, 200, map[string]string{"url": url})
}
func (a *App) portal(w http.ResponseWriter, r *http.Request, u User) error {
	if a.opts.Billing == nil {
		return unavailable()
	}
	url, err := a.opts.Billing.Portal(r.Context(), u.ID)
	if err != nil {
		return billingError(err)
	}
	return jsonOut(w, 200, map[string]string{"url": url})
}
func (a *App) invoices(w http.ResponseWriter, r *http.Request, u User) error {
	if a.opts.Billing == nil {
		return unavailable()
	}
	items, err := a.opts.Billing.Invoices(r.Context(), u.ID)
	if err != nil {
		return billingError(err)
	}
	return jsonOut(w, 200, map[string]any{"items": items})
}
func (a *App) webhook(w http.ResponseWriter, r *http.Request, _ User) error {
	if a.opts.Billing == nil {
		return unavailable()
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		return problem{413, "Événement trop volumineux."}
	}
	if err = a.opts.Billing.Webhook(r.Context(), body, r.Header.Get("Stripe-Signature")); err != nil {
		return billingError(err)
	}
	return empty(w, 204)
}
func (a *App) payments(w http.ResponseWriter, r *http.Request, _ User) error {
	if a.opts.Billing == nil {
		return unavailable()
	}
	rows, err := a.db.Query(r.Context(), `SELECT payment_intent_id,coalesce(email,''),amount,currency,refunded,p.created_at,coalesce(r.provider_status,'') FROM billing_payments p LEFT JOIN billing_refunds r USING(payment_intent_id) LEFT JOIN users u ON u.id=p.user_id ORDER BY p.created_at DESC`)
	if err != nil {
		return err
	}
	type payment struct {
		RefundStatus    string    `json:"refundStatus"`
		PaymentIntentID string    `json:"paymentIntentId"`
		Email           string    `json:"email"`
		Amount          int64     `json:"amount"`
		Currency        string    `json:"currency"`
		Refunded        bool      `json:"refunded"`
		CreatedAt       time.Time `json:"createdAt"`
	}
	items, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (payment, error) {
		var p payment
		err := r.Scan(&p.PaymentIntentID, &p.Email, &p.Amount, &p.Currency, &p.Refunded, &p.CreatedAt, &p.RefundStatus)
		return p, err
	})
	if err != nil {
		return err
	}
	return jsonOut(w, 200, map[string]any{"items": items})
}
func (a *App) refund(w http.ResponseWriter, r *http.Request, u User) error {
	if a.opts.Billing == nil {
		return unavailable()
	}
	var in struct {
		PaymentIntentID string `json:"paymentIntentId"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	id, err := a.opts.Billing.Refund(r.Context(), u.ID, in.PaymentIntentID)
	if err != nil {
		return billingError(err)
	}
	return jsonOut(w, 200, map[string]string{"refundId": id})
}
