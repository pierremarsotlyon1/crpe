// Package learning serves shared identity, teaching materials and student work.
package learning

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/mail"
	"net/netip"
	"net/url"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pierremarsotlyon1/crpe/backend/internal/billing"
	"github.com/pierremarsotlyon1/crpe/backend/internal/storebilling"
)

type Sender interface {
	Send(context.Context, string, string, string) error
}
type Store interface {
	Put(context.Context, string, io.Reader, int64, string) error
	Get(context.Context, string) (io.ReadCloser, string, error)
	Delete(context.Context, string) error
}
type Options struct {
	TrustedProxyCIDRs []netip.Prefix
	AuthIPHourlyLimit int
	Dialog            StructuredDialog
	Billing           *billing.Service
	Stores            *storebilling.Service
	Voice             Voice
	WebURL, MobileURL string
	AdminEmails       []string
	SalesEnabled      bool
	CorrectionEnabled bool
}
type App struct {
	db     *pgxpool.Pool
	mail   Sender
	store  Store
	opts   Options
	logger *slog.Logger
}

func New(db *pgxpool.Pool, mail Sender, store Store, opts Options, logger *slog.Logger) (*App, error) {
	if opts.AuthIPHourlyLimit == 0 {
		opts.AuthIPHourlyLimit = 120
	}
	if opts.AuthIPHourlyLimit < 1 {
		return nil, fmt.Errorf("invalid AUTH_IP_HOURLY_LIMIT")
	}
	u, err := url.Parse(opts.WebURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || net.ParseIP(u.Hostname()).IsLoopback()))) {
		return nil, fmt.Errorf("PUBLIC_WEB_URL requires HTTPS or loopback HTTP")
	}
	opts.WebURL = strings.TrimRight(opts.WebURL, "/")
	m, err := url.Parse(opts.MobileURL)
	if err != nil || m.Host == "" || m.User != nil || m.Fragment != "" || m.RawQuery != "" {
		return nil, fmt.Errorf("invalid MOBILE_AUTH_URL")
	}
	for i, e := range opts.AdminEmails {
		e = normalizeEmail(e)
		if e == "" {
			return nil, fmt.Errorf("invalid ADMIN_EMAILS entry")
		}
		opts.AdminEmails[i] = e
	}
	return &App{db: db, mail: mail, store: store, opts: opts, logger: logger}, nil
}

type User struct {
	SupportAccess bool   `json:"supportAccess,omitempty"`
	ID            string `json:"id"`
	Email         string `json:"email"`
	Role          string `json:"role"`
}
type endpoint func(http.ResponseWriter, *http.Request, User) error
type problem struct {
	code int
	msg  string
}

func (e problem) Error() string { return e.msg }
func bad(msg string) error      { return problem{400, msg} }
func missing() error            { return problem{404, "Ressource introuvable."} }
func unavailable() error        { return problem{503, "Service temporairement indisponible."} }
func (a *App) Register(mux *http.ServeMux) {
	protect := http.NewCrossOriginProtection()
	protect.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		_ = jsonOut(w, http.StatusForbidden, map[string]string{"error": "Origine de la requête non autorisée."})
	}))
	_ = protect.AddTrustedOrigin(a.opts.WebURL)
	route := func(pattern, access string, fn endpoint) {
		mux.Handle(pattern, protect.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			var u User
			var err error
			if access != "" {
				u, err = a.authenticate(r)
				if err == nil && u.Role == "admin" && !slices.Contains(a.opts.AdminEmails, u.Email) {
					u.Role = "student"
				}
				if err == nil && access == "support" && u.Role != "admin" && !u.SupportAccess {
					err = problem{403, "Cette activité est réservée aux membres Suivi."}
				}
				if err == nil && access == "learning" && a.opts.SalesEnabled && u.Role != "admin" {
					if a.opts.Billing == nil {
						err = unavailable()
					} else {
						var allowed bool
						allowed, err = a.opts.Billing.Access(r.Context(), u.ID)
						if err == nil && !allowed {
							err = problem{403, "Accès pédagogique requis."}
						}
					}
				}
				if err == nil && access == "admin" && u.Role != "admin" {
					err = problem{403, "Accès réservé à l’enseignante."}
				}
			}
			if err == nil {
				err = fn(w, r, u)
			}
			if err != nil {
				a.respondError(w, r, err)
			}
		})))
	}
	route("POST /v1/auth/request", "", a.requestLogin)
	route("POST /v1/auth/verify", "", a.verifyLogin)
	route("POST /v1/auth/logout", "member", a.logout)
	route("GET /v1/me", "member", func(w http.ResponseWriter, r *http.Request, u User) error { return jsonOut(w, 200, u) })
	route("DELETE /v1/me", "member", a.deleteAccount)
	route("GET /v1/me/export", "member", a.exportAccount)
	route("POST /v1/waitlist", "", a.joinWaitlist)
	route("POST /v1/waitlist/confirm", "", a.confirmWaitlist)
	route("POST /v1/waitlist/unsubscribe", "", a.unsubscribe)
	route("GET /v1/catalog", "learning", a.catalog)
	route("GET /v1/public/subjects", "", a.publicSubjects)
	route("GET /v1/mock-sessions", "member", a.supportMocks)
	route("GET /v1/me/access", "member", a.access)
	route("GET /v1/me/store-products", "member", a.products)
	route("POST /v1/me/store-orders", "member", a.reserveOrder)
	route("DELETE /v1/me/store-orders/{platform}", "member", a.cancelOrder)
	route("POST /v1/me/store-purchases", "member", a.verifyPurchase)
	route("POST /v1/store-notifications/{platform}", "", a.notifyStore)
	route("POST /v1/mock-sessions", "learning", a.startMock)
	route("GET /v1/mock-sessions/{id}", "learning", a.mock)
	route("PUT /v1/mock-sessions/{id}/answers", "learning", a.mock)
	route("POST /v1/mock-sessions/{id}/finish", "learning", a.mock)
	route("GET /v1/revision-plan", "learning", a.revisionPlan)
	route("POST /v1/oral-sessions", "learning", a.startOral)
	route("GET /v1/oral-sessions", "member", a.listOral)
	route("POST /v1/oral-sessions/{id}/finish", "learning", a.finishOral)
	route("POST /v1/oral-sessions/{id}/transcribe", "learning", a.transcribe)
	route("POST /v1/oral-sessions/{id}/speech", "learning", a.speak)
	route("GET /v1/admin/oral-sessions", "admin", a.listOral)
	route("POST /v1/admin/oral-sessions/{id}/review", "admin", a.reviewOral)
	route("GET /v1/practice", "learning", a.practice)
	route("GET /v1/progress", "member", a.progress)
	route("POST /v1/attempts", "learning", a.attempt)
	route("GET /v1/admin/content", "admin", a.adminCatalog)
	route("POST /v1/admin/content", "admin", a.saveContent)
	route("PUT /v1/admin/content/{id}", "admin", a.saveContent)
	route("POST /v1/admin/content/{id}/validate", "admin", a.publish)
	route("DELETE /v1/admin/content/{id}", "admin", a.archive)
	route("POST /v1/admin/mock-drafts", "admin", a.mockDraft)
	route("POST /v1/admin/import", "admin", a.importContent)
	route("POST /v1/admin/content/{id}/documents", "admin", a.addDocument)
	route("DELETE /v1/admin/documents/{id}", "admin", a.deleteDocument)
	route("GET /v1/documents/{id}/file", "learning", a.document)
	route("GET /v1/billing/plans", "", a.plans)
	route("POST /v1/billing/checkout", "member", a.checkout)
	route("POST /v1/billing/portal", "member", a.portal)
	route("GET /v1/billing/invoices", "member", a.invoices)
	route("POST /v1/billing/webhook", "", a.webhook)
	route("GET /v1/admin/payments", "admin", a.payments)
	route("POST /v1/admin/refunds", "admin", a.refund)
	route("GET /v1/support", "support", a.support)
	route("GET /v1/admin/support", "admin", a.support)
	route("PUT /v1/admin/support", "admin", a.updateSupport)
	route("GET /v1/admin/mock-sessions", "admin", a.supportMocks)
	route("GET /v1/admin/mock-sessions/{id}", "admin", a.teacherMock)
	route("POST /v1/admin/mock-sessions/{id}/review", "admin", a.teacherMockReview)
	route("GET /v1/admin/stats", "admin", a.stats)
	route("POST /v1/submissions", "learning", a.submit)
	route("GET /v1/submissions", "member", a.submissions)
	route("GET /v1/admin/submissions", "admin", a.submissions)
	route("POST /v1/admin/submissions/{id}/review", "admin", a.review)
	route("GET /v1/submissions/{id}", "member", a.submissionDetail)
	route("GET /v1/submissions/{id}/image", "member", a.submissionImage)
}
func (a *App) respondError(w http.ResponseWriter, r *http.Request, err error) {
	var p problem
	var dbErr *pgconn.PgError
	switch {
	case errors.As(err, &p):
	case errors.Is(err, pgx.ErrNoRows):
		p = missing().(problem)
	case errors.As(err, &dbErr) && (dbErr.Code == "23503" || dbErr.Code == "23505"):
		p = problem{409, "La ressource a changé."}
	default:
		a.logger.Error("request failed", "route", r.Pattern, "type", fmt.Sprintf("%T", err))
		p = unavailable().(problem)
	}
	_ = jsonOut(w, p.code, map[string]string{"error": p.msg})
}
func jsonOut(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(v)
}
func empty(w http.ResponseWriter, status int) error { w.WriteHeader(status); return nil }
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "application/json" {
		return problem{415, "Format JSON attendu."}
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return bad("Requête JSON invalide.")
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return bad("Requête JSON invalide.")
	}
	return nil
}
func normalizeEmail(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	m, err := mail.ParseAddress(s)
	if err != nil || m.Address != s || len(s) > 254 || strings.ContainsAny(s, "\r\n") {
		return ""
	}
	return s
}
func token() string {
	var b [32]byte
	rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}
func hash(s string) []byte { v := sha256.Sum256([]byte(s)); return v[:] }
func uuid(s string) bool {
	if len(s) != 36 || strings.Count(s, "-") != 4 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return false
	}
	_, err := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
	return err == nil
}
func pathID(r *http.Request) (string, error) {
	id := r.PathValue("id")
	if !uuid(id) {
		return "", missing()
	}
	return id, nil
}
func (a *App) rate(ctx context.Context, key string, limit int) error {
	var n int
	// One request across all replicas performs expiry cleanup per UTC hour.
	err := a.db.QueryRow(ctx, `WITH bucket AS (SELECT floor(extract(epoch FROM now())/3600)::bigint AS hour),
 tick AS (INSERT INTO rate_limits(key,bucket,count) SELECT $3,hour,1 FROM bucket ON CONFLICT DO NOTHING RETURNING 1),
 old_limits AS (DELETE FROM rate_limits WHERE bucket<(SELECT hour FROM bucket)-1 AND EXISTS(SELECT FROM tick)),
 old_links AS (DELETE FROM auth_links WHERE expires_at<=now() AND EXISTS(SELECT FROM tick)),
 old_sessions AS (DELETE FROM sessions WHERE expires_at<=now() AND EXISTS(SELECT FROM tick))
 INSERT INTO rate_limits(key,bucket,count) SELECT $1,hour,1 FROM bucket
 ON CONFLICT(bucket,key) DO UPDATE SET count=rate_limits.count+1 WHERE rate_limits.count<$2 RETURNING count`, hash(key), limit, hash("maintenance/expiry")).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return problem{429, "Limite atteinte. Réessayez plus tard."}
	}
	return err
}
func (a *App) requestIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err == nil {
		for _, network := range a.opts.TrustedProxyCIDRs {
			if network.Contains(peer.Unmap()) {
				real, err := netip.ParseAddr(r.Header.Get("X-Real-IP"))
				if err == nil {
					return real.Unmap().String()
				}
				break
			}
		}
	}
	return host
}
func transaction(ctx context.Context, db *pgxpool.Pool, fn func(pgx.Tx) error) error {
	return pgx.BeginFunc(ctx, db, fn)
}
func ttlCookie(a *App, v string, seconds int) *http.Cookie {
	return &http.Cookie{Name: "crpe_session", Value: v, Path: "/", MaxAge: seconds, HttpOnly: true, Secure: strings.HasPrefix(a.opts.WebURL, "https://"), SameSite: http.SameSiteLaxMode}
}
func sessionKey(r *http.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		v, ok := strings.CutPrefix(h, "Bearer ")
		if ok {
			return v
		}
		return ""
	}
	c, err := r.Cookie("crpe_session")
	if err == nil {
		return c.Value
	}
	return ""
}
func (a *App) authenticate(r *http.Request) (User, error) {
	var u User
	s := sessionKey(r)
	if len(s) != 43 {
		return u, problem{401, "Connexion requise."}
	}
	err := a.db.QueryRow(r.Context(), `SELECT u.id::text,email,role,EXISTS(SELECT FROM member_rights e WHERE e.user_id=u.id AND (e.tier='suivi' OR (e.source NOT IN ('apple','google') AND e.plan_key=ANY($2::text[]))) AND e.valid_until>now() AND NOT e.refunded) FROM sessions s JOIN users u ON u.id=s.user_id WHERE token_hash=$1 AND expires_at>now()`, hash(s), a.opts.Billing.SupportPlans()).Scan(&u.ID, &u.Email, &u.Role, &u.SupportAccess)
	if errors.Is(err, pgx.ErrNoRows) {
		err = problem{401, "Session expirée."}
	}
	return u, err
}
