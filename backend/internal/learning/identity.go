package learning

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"

	"github.com/jackc/pgx/v5"
)

type linkRequest struct {
	Email  string `json:"email"`
	Client string `json:"client"`
}
type verifyRequest struct {
	Token  string `json:"token"`
	Client string `json:"client"`
}

func checkClient(s string) (string, error) {
	if s == "" || s == "web" {
		return "web", nil
	}
	if s == "mobile" {
		return s, nil
	}
	return "", bad("Client inconnu.")
}
func (a *App) requestLogin(w http.ResponseWriter, r *http.Request, _ User) error {
	var in linkRequest
	if err := decode(w, r, &in); err != nil {
		return err
	}
	email := normalizeEmail(in.Email)
	client, err := checkClient(in.Client)
	if email == "" {
		return bad("Adresse email invalide.")
	}
	if err != nil {
		return err
	}
	ctx := r.Context()
	if err = a.rate(ctx, "auth-ip/"+a.requestIP(r), a.opts.AuthIPHourlyLimit); err != nil {
		return err
	}
	if err = a.rate(ctx, "auth/"+email, 6); err != nil {
		return err
	}
	if a.mail == nil {
		return unavailable()
	}
	secret := token()
	if _, err = a.db.Exec(ctx, "INSERT INTO auth_links VALUES($1,$2,$3,now()+interval '15 minutes')", hash(secret), email, client); err != nil {
		return err
	}
	url := a.opts.WebURL + "/connexion/confirmer"
	if client == "mobile" {
		url = a.opts.MobileURL
	}
	if err = a.mail.Send(ctx, email, "Connexion à Prépa Maths CRPE", "Ce lien personnel est valable 15 minutes.\n\n"+url+"#token="+secret); err != nil {
		return unavailable()
	}
	return empty(w, 202)
}
func (a *App) verifyLogin(w http.ResponseWriter, r *http.Request, _ User) error {
	var in verifyRequest
	if err := decode(w, r, &in); err != nil {
		return err
	}
	client, err := checkClient(in.Client)
	if err != nil {
		return err
	}
	if len(in.Token) != 43 {
		return bad("Lien invalide ou expiré.")
	}
	var u User
	session := token()
	ctx := r.Context()
	err = transaction(ctx, a.db, func(tx pgx.Tx) error {
		var email string
		err := tx.QueryRow(ctx, "DELETE FROM auth_links WHERE token_hash=$1 AND client=$2 AND expires_at>now() RETURNING email", hash(in.Token), client).Scan(&email)
		if errors.Is(err, pgx.ErrNoRows) {
			return bad("Lien invalide ou expiré.")
		}
		if err != nil {
			return err
		}
		role := "student"
		if slices.Contains(a.opts.AdminEmails, email) {
			role = "admin"
		}
		err = tx.QueryRow(ctx, `INSERT INTO users(email,role) VALUES($1,$2) ON CONFLICT(email) DO UPDATE SET role=EXCLUDED.role RETURNING id::text,email,role`, email, role).Scan(&u.ID, &u.Email, &u.Role)
		if err != nil {
			return err
		}
		if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT FROM member_rights WHERE user_id=$1 AND (tier='suivi' OR (source NOT IN ('apple','google') AND plan_key=ANY($2::text[]))) AND valid_until>now() AND NOT refunded)", u.ID, a.opts.Billing.SupportPlans()).Scan(&u.SupportAccess); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "INSERT INTO sessions VALUES($1,$2,now()+interval '30 days')", hash(session), u.ID)
		return err
	})
	if err != nil {
		return err
	}
	out := map[string]any{"user": u}
	if client == "mobile" {
		out["token"] = session
	} else {
		http.SetCookie(w, ttlCookie(a, session, 30*86400))
	}
	return jsonOut(w, 200, out)
}
func (a *App) logout(w http.ResponseWriter, r *http.Request, _ User) error {
	if _, err := a.db.Exec(r.Context(), "DELETE FROM sessions WHERE token_hash=$1", hash(sessionKey(r))); err != nil {
		return err
	}
	http.SetCookie(w, ttlCookie(a, "", -1))
	return empty(w, 204)
}
func (a *App) joinWaitlist(w http.ResponseWriter, r *http.Request, _ User) error {
	var in struct {
		Email   string `json:"email"`
		Consent bool   `json:"consent"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	email := normalizeEmail(in.Email)
	if email == "" || !in.Consent {
		return bad("Une adresse valide et votre consentement sont nécessaires.")
	}
	ctx := r.Context()
	if err := a.rate(ctx, "waitlist-ip/"+a.requestIP(r), a.opts.AuthIPHourlyLimit); err != nil {
		return err
	}
	if err := a.rate(ctx, "waitlist/"+email, 5); err != nil {
		return err
	}
	if a.mail == nil {
		return unavailable()
	}
	confirm, leave := token(), token()
	err := transaction(ctx, a.db, func(tx pgx.Tx) error {
		var existing string
		err := tx.QueryRow(ctx, `INSERT INTO waitlist(email,confirm_hash,confirm_expires,unsubscribe_hash) VALUES($1,$2,now()+interval '48 hours',$3)
 ON CONFLICT(email) DO UPDATE SET confirm_hash=EXCLUDED.confirm_hash,confirm_expires=EXCLUDED.confirm_expires,unsubscribe_hash=EXCLUDED.unsubscribe_hash WHERE waitlist.confirmed_at IS NULL RETURNING email`, email, hash(confirm), hash(leave)).Scan(&existing)
		if err != nil {
			return err
		}
		return consent(ctx, tx, email, "waitlist_request")
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return empty(w, 202)
	}
	if err != nil {
		return err
	}
	body := "Confirmez votre inscription sous 48 heures :\n" + a.opts.WebURL + "/liste-attente/confirmer#token=" + confirm + "\n\nPour retirer votre inscription :\n" + a.opts.WebURL + "/liste-attente/desinscrire#token=" + leave
	if err = a.mail.Send(ctx, email, "Confirmez votre inscription à la liste d’attente", body); err != nil {
		return unavailable()
	}
	return empty(w, 202)
}
func consent(ctx context.Context, tx pgx.Tx, email, event string) error {
	_, err := tx.Exec(ctx, "INSERT INTO consents(email,event,policy_version) VALUES($1,$2,'waitlist-v1')", email, event)
	return err
}
func (a *App) confirmWaitlist(w http.ResponseWriter, r *http.Request, _ User) error {
	return a.changeWaitlist(w, r, false)
}
func (a *App) unsubscribe(w http.ResponseWriter, r *http.Request, _ User) error {
	return a.changeWaitlist(w, r, true)
}
func (a *App) changeWaitlist(w http.ResponseWriter, r *http.Request, leave bool) error {
	var in struct {
		Token string `json:"token"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	if len(in.Token) != 43 {
		return bad("Lien invalide ou expiré.")
	}
	ctx := r.Context()
	err := transaction(ctx, a.db, func(tx pgx.Tx) error {
		var email string
		sql := "UPDATE waitlist SET confirmed_at=now(),confirm_hash=NULL,confirm_expires=NULL WHERE confirm_hash=$1 AND confirm_expires>now() RETURNING email"
		event := "waitlist_confirm"
		if leave {
			sql = "DELETE FROM waitlist WHERE unsubscribe_hash=$1 RETURNING email"
			event = "waitlist_unsubscribe"
		}
		if err := tx.QueryRow(ctx, sql, hash(in.Token)).Scan(&email); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return bad("Lien invalide ou expiré.")
			}
			return err
		}
		return consent(ctx, tx, email, event)
	})
	if err != nil {
		return err
	}
	return empty(w, 204)
}
func (a *App) exportAccount(w http.ResponseWriter, r *http.Request, u User) error {
	// Export a single consistent snapshot, excluding credentials and internal object keys.
	var raw json.RawMessage
	err := a.db.QueryRow(r.Context(), `SELECT jsonb_build_object('user',to_jsonb(u),
 'attempts',COALESCE((SELECT jsonb_agg(to_jsonb(a)-'user_id') FROM attempts a WHERE a.user_id=u.id),'[]'),
 'submissions',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',s.id,'exerciseId',s.exercise_id,'text',s.text_answer,'status',s.status,'feedback',s.feedback,'reviewVideoUrl',(SELECT review_video_url FROM submission_video_reviews v WHERE v.submission_id=s.id),'correction',s.correction,'createdAt',s.created_at,'imagePath',CASE WHEN object_key<>'' THEN '/v1/submissions/'||s.id||'/image' END)) FROM submissions s WHERE s.user_id=u.id),'[]'),
 'mockSessions',COALESCE((SELECT jsonb_agg(to_jsonb(m)-ARRAY['user_id','questions'] || jsonb_build_object('teacherReview',(SELECT to_jsonb(v)-'session_id' FROM mock_video_reviews v WHERE v.session_id=m.id))) FROM mock_sessions m WHERE m.user_id=u.id),'[]'),
 'oralSessions',COALESCE((SELECT jsonb_agg(to_jsonb(o)-'user_id') FROM oral_sessions o WHERE o.user_id=u.id),'[]'),
 'entitlements',COALESCE((SELECT jsonb_agg(jsonb_build_object('provider',source,'plan',plan_key,'validUntil',valid_until,'refunded',refunded)) FROM member_rights e WHERE e.user_id=u.id),'[]'),
 'payments',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',payment_intent_id,'amount',amount,'currency',currency,'refunded',refunded,'date',created_at)) FROM billing_payments p WHERE p.user_id=u.id),'[]'),
 'consents',COALESCE((SELECT jsonb_agg(jsonb_build_object('event',event,'version',policy_version,'at',recorded_at)) FROM consents WHERE email=u.email),'[]'),
 'waitlist', (SELECT jsonb_build_object('confirmedAt',confirmed_at) FROM waitlist WHERE email=u.email))
 FROM users u WHERE u.id=$1`, u.ID).Scan(&raw)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Disposition", `attachment; filename="mes-donnees-crpe.json"`)
	return jsonOut(w, 200, raw)
}
func (a *App) deleteAccount(w http.ResponseWriter, r *http.Request, u User) error {
	ctx := r.Context()
	err := transaction(ctx, a.db, func(tx pgx.Tx) error {
		var email string
		if err := tx.QueryRow(ctx, "SELECT email FROM users WHERE id=$1 FOR UPDATE", u.ID).Scan(&email); err != nil {
			return err
		}
		if a.opts.Billing != nil {
			if err := a.opts.Billing.CloseAccount(ctx, u.ID, tx); err != nil {
				return unavailable()
			}
		}
		_, err := tx.Exec(ctx, `INSERT INTO jobs(kind,payload,deduplication_key) SELECT 'delete_object',jsonb_build_object('key',object_key),'delete/'||object_key FROM submissions WHERE user_id=$1 AND object_key<>'' ON CONFLICT DO NOTHING`, u.ID)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE store_memberships SET lookup_cipher=''::bytea WHERE user_id=$1`, u.ID); err != nil {
			return err
		}
		for _, q := range []string{"DELETE FROM auth_links WHERE email=$1", "DELETE FROM waitlist WHERE email=$1", "DELETE FROM consents WHERE email=$1", "DELETE FROM users WHERE email=$1"} {
			if _, err = tx.Exec(ctx, q, email); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	http.SetCookie(w, ttlCookie(a, "", -1))
	return empty(w, 204)
}
