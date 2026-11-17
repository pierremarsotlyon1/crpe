package learning

import (
	"errors"
	"io"
	"net/http"

	"github.com/pierremarsotlyon1/crpe/backend/internal/storebilling"
	"github.com/pierremarsotlyon1/crpe/backend/internal/storeprovider"
)

func storeError(e error) error {
	switch {
	case errors.Is(e, storebilling.ErrInvalid), errors.Is(e, storeprovider.ErrInvalid):
		return bad("Achat ou produit indisponible.")
	case errors.Is(e, storebilling.ErrOwner):
		return problem{403, "Cet achat est lié à un autre compte."}
	case errors.Is(e, storebilling.ErrConflict):
		return problem{409, "Un abonnement ou une commande existe déjà auprès d’un autre fournisseur."}
	case errors.Is(e, storebilling.ErrDisabled):
		return missing()
	case errors.Is(e, storeprovider.ErrUnavailable):
		return unavailable()
	}
	return billingError(e)
}
func (a *App) products(w http.ResponseWriter, r *http.Request, u User) error {
	if a.opts.Stores == nil {
		return unavailable()
	}
	c, e := a.opts.Stores.Catalog(r.Context(), u.ID, r.URL.Query().Get("platform"), a.opts.WebURL, a.opts.SalesEnabled, a.opts.Billing.SupportPlans())
	if e != nil {
		return storeError(e)
	}
	return jsonOut(w, 200, c)
}
func (a *App) reserveOrder(w http.ResponseWriter, r *http.Request, u User) error {
	if a.opts.Stores == nil {
		return unavailable()
	}
	var in storebilling.OrderInput
	if e := decode(w, r, &in); e != nil {
		return e
	}
	if e := a.rate(r.Context(), "store-order/"+u.ID, 30); e != nil {
		return e
	}
	order, e := a.opts.Stores.Reserve(r.Context(), u.ID, in, a.opts.SalesEnabled, a.opts.Billing)
	if e != nil {
		return storeError(e)
	}
	return jsonOut(w, 200, order)
}
func (a *App) cancelOrder(w http.ResponseWriter, r *http.Request, u User) error {
	if a.opts.Stores == nil {
		return unavailable()
	}
	if e := a.opts.Stores.Cancel(r.Context(), u.ID, r.PathValue("platform"), r.URL.Query().Get("orderId")); e != nil {
		return storeError(e)
	}
	return empty(w, 204)
}
func (a *App) verifyPurchase(w http.ResponseWriter, r *http.Request, u User) error {
	if a.opts.Stores == nil {
		return unavailable()
	}
	var in struct {
		Platform  string `json:"platform"`
		ProductID string `json:"productId"`
		Reference string `json:"reference"`
	}
	if e := decode(w, r, &in); e != nil {
		return e
	}
	if e := a.rate(r.Context(), "store-verify/"+u.ID, 120); e != nil {
		return e
	}
	receipt, e := a.opts.Stores.Purchase(r.Context(), u.ID, in.Platform, in.ProductID, in.Reference)
	if e != nil {
		return storeError(e)
	}
	return jsonOut(w, 200, receipt)
}
func (a *App) notifyStore(w http.ResponseWriter, r *http.Request, _ User) error {
	if a.opts.Stores == nil {
		return unavailable()
	}
	body, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 512<<10))
	if e != nil {
		return problem{413, "Événement trop volumineux."}
	}
	if e = a.opts.Stores.Notify(r.Context(), r.PathValue("platform"), r.Header, body); e != nil {
		return storeError(e)
	}
	return empty(w, 204)
}
