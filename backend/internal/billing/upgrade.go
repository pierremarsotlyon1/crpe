package billing

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
)

type activePlan struct{ key, source, customer, subscription string }

// prepareUpgrade returns a client-confirmed portal change for one existing Stripe
// subscription. Only a verified webhook changes local access.
func (s *Service) prepareUpgrade(ctx context.Context, tx pgx.Tx, user string, target planConfig) (string, bool, error) {
	rows, err := tx.Query(ctx, `SELECT plan_key,source,stripe_customer_id,stripe_subscription_id FROM entitlements WHERE user_id=$1 AND valid_until>now() AND NOT refunded LIMIT 2`, user)
	if err != nil {
		return "", false, err
	}
	plans, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (activePlan, error) {
		var p activePlan
		e := r.Scan(&p.key, &p.source, &p.customer, &p.subscription)
		return p, e
	})
	if err != nil || len(plans) == 0 {
		return "", false, err
	}
	if len(plans) != 1 || target.Tier != "suivi" {
		return "", true, ErrActiveSubscription
	}
	p := plans[0]
	if p.source != "stripe" || !strings.HasPrefix(p.subscription, "sub_") || !strings.HasPrefix(p.customer, "cus_") {
		return "", true, ErrActiveSubscription
	}
	currentPrice := ""
	for _, cfg := range s.plans {
		if cfg.Key == p.key && cfg.Tier == "autonomie" {
			currentPrice = cfg.PriceID
			break
		}
	}
	if currentPrice == "" {
		return "", true, ErrActiveSubscription
	}
	var sub struct {
		ID, Customer, Status string
		Items                struct {
			Data []struct {
				ID    string
				Price struct{ ID string }
			}
		}
	}
	if err = s.request(ctx, http.MethodGet, "/v1/subscriptions/"+url.PathEscape(p.subscription), nil, "", &sub); err != nil {
		return "", true, err
	}
	if sub.ID != p.subscription || sub.Customer != p.customer || sub.Status != "active" && sub.Status != "trialing" || len(sub.Items.Data) != 1 || sub.Items.Data[0].Price.ID != currentPrice || !strings.HasPrefix(sub.Items.Data[0].ID, "si_") {
		return "", true, ErrActiveSubscription
	}
	back := s.webURL + "/espace/abonnement"
	form := url.Values{"customer": {p.customer}, "return_url": {back}, "locale": {"fr"},
		"flow_data[type]": {"subscription_update_confirm"}, "flow_data[subscription_update_confirm][subscription]": {p.subscription},
		"flow_data[subscription_update_confirm][items][0][id]": {sub.Items.Data[0].ID}, "flow_data[subscription_update_confirm][items][0][price]": {target.PriceID},
		"flow_data[after_completion][type]": {"redirect"}, "flow_data[after_completion][redirect][return_url]": {back + "?retour=succes"}}
	var out struct{ URL string }
	if err = s.request(ctx, http.MethodPost, "/v1/billing_portal/sessions", form, "", &out); err != nil {
		return "", true, err
	}
	if !strings.HasPrefix(out.URL, "https://billing.stripe.com/") {
		return "", true, fmt.Errorf("invalid upgrade portal URL")
	}
	return out.URL, true, nil
}
