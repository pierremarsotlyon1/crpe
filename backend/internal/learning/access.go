package learning

import (
	"net/http"
	"net/url"
	"time"

	"github.com/pierremarsotlyon1/crpe/backend/internal/storebilling"
)

type capability struct {
	Key          string `json:"key"`
	Label        string `json:"label"`
	RequiredTier string `json:"requiredTier"`
	Allowed      bool   `json:"allowed"`
	UpgradeURL   string `json:"upgradeUrl,omitempty"`
}

func (a *App) access(w http.ResponseWriter, r *http.Request, u User) error {
	tier := "none"
	if u.Role == "admin" || u.SupportAccess {
		tier = "suivi"
	} else if !a.opts.SalesEnabled {
		tier = "autonomie"
	} else if a.opts.Billing != nil {
		active, err := a.opts.Billing.Access(r.Context(), u.ID)
		if err != nil {
			return err
		}
		if active {
			tier = "autonomie"
		}
	}
	out := struct {
		Tier         string                        `json:"tier"`
		SalesEnabled bool                          `json:"salesEnabled"`
		Features     []capability                  `json:"features"`
		UpgradeURL   string                        `json:"upgradeUrl,omitempty"`
		Providers    []storebilling.ActiveProvider `json:"providers"`
		ValidUntil   *time.Time                    `json:"validUntil,omitempty"`
	}{Tier: tier, SalesEnabled: a.opts.SalesEnabled}
	var err error
	out.Providers, err = storebilling.Active(r.Context(), a.db, u.ID, a.opts.Billing.SupportPlans())
	if err != nil {
		return err
	}
	if u.Role != "admin" && (tier == "suivi" || tier == "autonomie" && a.opts.SalesEnabled) {
		err = a.db.QueryRow(r.Context(), `SELECT max(valid_until) FROM member_rights WHERE user_id=$1 AND NOT refunded AND valid_until>now() AND ($2<>'suivi' OR tier='suivi' OR (source NOT IN('apple','google') AND plan_key=ANY($3::text[])))`, u.ID, tier, a.opts.Billing.SupportPlans()).Scan(&out.ValidUntil)
		if err != nil {
			return err
		}
	}
	for _, f := range []capability{{Key: "course", Label: "Parcours de cours", RequiredTier: "autonomie"}, {Key: "practice", Label: "Entraînement quotidien", RequiredTier: "autonomie"}, {Key: "tutor", Label: "Tuteur de correction", RequiredTier: "autonomie"}, {Key: "mock", Label: "Sujets blancs", RequiredTier: "autonomie"}, {Key: "oral", Label: "Entraînement à l’oral", RequiredTier: "autonomie"}, {Key: "live", Label: "Rendez-vous hebdomadaire", RequiredTier: "suivi"}, {Key: "mock_video", Label: "Correction vidéo des sujets blancs", RequiredTier: "suivi"}, {Key: "group", Label: "Groupe WhatsApp", RequiredTier: "suivi"}} {
		f.Allowed = tier == "suivi" || tier == "autonomie" && f.RequiredTier == "autonomie"
		if !f.Allowed && a.opts.SalesEnabled {
			q := url.Values{"feature": {f.Key}, "tier": {f.RequiredTier}}
			f.UpgradeURL = a.opts.WebURL + "/offres?" + q.Encode()
			if out.UpgradeURL == "" {
				out.UpgradeURL = f.UpgradeURL
			}
		}
		out.Features = append(out.Features, f)
	}
	return jsonOut(w, 200, out)
}
