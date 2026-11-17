package learning

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type supportSettings struct {
	LiveURL      string `json:"liveUrl"`
	LiveSchedule string `json:"liveSchedule"`
	GroupURL     string `json:"groupUrl"`
}
type mockReview struct {
	SupportAccess  bool      `json:"supportAccess"`
	GradingStatus  string    `json:"gradingStatus"`
	ID             string    `json:"id"`
	SubjectID      string    `json:"subjectId"`
	Title          string    `json:"title"`
	Email          string    `json:"email"`
	StartedAt      time.Time `json:"startedAt"`
	Feedback       string    `json:"feedback"`
	ReviewVideoURL string    `json:"reviewVideoUrl"`
}

func safeLink(s string) bool {
	if s == "" {
		return true
	}
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && len(s) <= 2048
}
func (a *App) support(w http.ResponseWriter, r *http.Request, u User) error {
	var v supportSettings
	ctx := r.Context()
	if err := a.db.QueryRow(ctx, "SELECT live_url,live_schedule,group_url FROM support_settings").Scan(&v.LiveURL, &v.LiveSchedule, &v.GroupURL); err != nil {
		return err
	}
	if strings.HasPrefix(r.URL.Path, "/v1/admin/") {
		return jsonOut(w, 200, v)
	}
	items, err := a.mockReviews(ctx, a.db, u.ID, "")
	if err != nil {
		return err
	}
	return jsonOut(w, 200, struct {
		supportSettings
		MockReviews []mockReview `json:"mockReviews"`
	}{v, items})
}
func (a *App) updateSupport(w http.ResponseWriter, r *http.Request, _ User) error {
	var v supportSettings
	if err := decode(w, r, &v); err != nil {
		return err
	}
	v.LiveURL = strings.TrimSpace(v.LiveURL)
	v.GroupURL = strings.TrimSpace(v.GroupURL)
	v.LiveSchedule = strings.TrimSpace(v.LiveSchedule)
	if !safeLink(v.LiveURL) || !safeLink(v.GroupURL) || len(v.LiveSchedule) > 600 || v.LiveURL != "" && v.LiveSchedule == "" {
		return bad("Renseignez une URL HTTPS et l’horaire du rendez-vous hebdomadaire.")
	}
	if v.GroupURL != "" {
		g, _ := url.Parse(v.GroupURL)
		if !strings.EqualFold(g.Host, "chat.whatsapp.com") || strings.Trim(g.Path, "/") == "" {
			return bad("Utilisez le lien d’invitation du groupe WhatsApp.")
		}
	}
	_, err := a.db.Exec(r.Context(), "UPDATE support_settings SET live_url=$1,live_schedule=$2,group_url=$3", v.LiveURL, v.LiveSchedule, v.GroupURL)
	if err != nil {
		return err
	}
	return jsonOut(w, 200, v)
}
func (a *App) mockReviews(ctx context.Context, q queryer, owner, id string) ([]mockReview, error) {
	rows, err := q.Query(ctx, `SELECT m.id::text,m.subject_id::text,c.title,u.email,m.started_at,coalesce(v.feedback,''),CASE WHEN access.eligible THEN coalesce(v.review_video_url,'') ELSE '' END,access.eligible,CASE WHEN m.needs_review THEN 'pending_review' ELSE 'complete' END
 FROM mock_sessions m JOIN users u ON u.id=m.user_id JOIN content c ON c.id=m.subject_id LEFT JOIN mock_video_reviews v ON v.session_id=m.id
 CROSS JOIN LATERAL (SELECT EXISTS(SELECT FROM member_rights e WHERE e.user_id=m.user_id AND (e.tier='suivi' OR (e.source NOT IN ('apple','google') AND e.plan_key=ANY($3::text[]))) AND e.valid_until>now() AND NOT e.refunded) AS eligible) access
 WHERE m.completed AND (nullif($1,'')::uuid IS NULL OR m.user_id=nullif($1,'')::uuid) AND (nullif($2,'')::uuid IS NULL OR m.id=nullif($2,'')::uuid)
 AND (nullif($1,'')::uuid IS NOT NULL OR access.eligible OR m.needs_review OR v.session_id IS NOT NULL) ORDER BY m.started_at DESC`, owner, id, a.opts.Billing.SupportPlans())
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (mockReview, error) {
		var v mockReview
		err := r.Scan(&v.ID, &v.SubjectID, &v.Title, &v.Email, &v.StartedAt, &v.Feedback, &v.ReviewVideoURL, &v.SupportAccess, &v.GradingStatus)
		return v, err
	})
}
func (a *App) supportMocks(w http.ResponseWriter, r *http.Request, u User) error {
	owner := u.ID
	if strings.HasPrefix(r.URL.Path, "/v1/admin/") {
		owner = ""
	}
	items, err := a.mockReviews(r.Context(), a.db, owner, "")
	if err != nil {
		return err
	}
	return jsonOut(w, 200, map[string]any{"items": items})
}
func (a *App) teacherMock(w http.ResponseWriter, r *http.Request, _ User) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	var out mockSession
	err = transaction(ctx, a.db, func(tx pgx.Tx) error {
		uid, eligible, e := a.reviewableMock(ctx, tx, id)
		if e != nil {
			return e
		}
		out, e = loadMock(ctx, tx, uid, id, nil, false, eligible)
		return e
	})
	if err != nil {
		return err
	}
	return jsonOut(w, 200, out)
}
func (a *App) reviewableMock(ctx context.Context, tx pgx.Tx, id string) (string, bool, error) {
	var uid string
	var eligible, needs, wasReviewed bool
	err := tx.QueryRow(ctx, `SELECT m.user_id::text,m.needs_review,EXISTS(SELECT FROM mock_video_reviews x WHERE x.session_id=m.id),
 EXISTS(SELECT FROM member_rights e WHERE e.user_id=m.user_id AND e.valid_until>now() AND NOT e.refunded AND (e.tier='suivi' OR (e.source NOT IN ('apple','google') AND e.plan_key=ANY($2::text[]))))
 FROM mock_sessions m WHERE m.id=$1 AND m.completed FOR UPDATE`, id, a.opts.Billing.SupportPlans()).Scan(&uid, &needs, &wasReviewed, &eligible)
	if err == nil && !eligible && !needs && !wasReviewed {
		return "", false, missing()
	}
	return uid, eligible, err
}

type assessment struct {
	ExerciseID string   `json:"exerciseId"`
	Points     *float64 `json:"points"`
	Feedback   string   `json:"feedback"`
}

func assessWritten(s *mockSession, assessments []assessment) (bool, error) {
	pending := false
	byID := make(map[string]*mockResult, len(s.Results))
	for i := range s.Results {
		byID[s.Results[i].ExerciseID] = &s.Results[i]
	}
	seen := map[string]bool{}
	for _, a := range assessments {
		result := byID[a.ExerciseID]
		if result == nil || result.Correct != nil || a.Points == nil || !finite(*a.Points) || *a.Points < 0 || *a.Points > result.MaxPoints || len(a.Feedback) > 16000 || seen[a.ExerciseID] {
			return false, bad("Évaluation écrite invalide : vérifiez l’exercice et le barème.")
		}
		seen[a.ExerciseID] = true
		result.Points = a.Points
		result.TeacherFeedback = a.Feedback
		result.GradingStatus = "complete"
	}
	for _, result := range s.Results {
		if result.GradingStatus == "pending_review" {
			pending = true
			break
		}
	}
	return pending, nil
}
func (a *App) teacherMockReview(w http.ResponseWriter, r *http.Request, _ User) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var in struct {
		Feedback       string       `json:"feedback"`
		ReviewVideoURL *string      `json:"reviewVideoUrl"`
		Assessments    []assessment `json:"assessments"`
	}
	if err = decode(w, r, &in); err != nil {
		return err
	}
	in.Feedback = strings.TrimSpace(in.Feedback)
	if len(in.Feedback) > 32000 || len(in.Assessments) > 100 || in.ReviewVideoURL != nil && !safeLink(*in.ReviewVideoURL) || in.Feedback == "" && len(in.Assessments) == 0 && in.ReviewVideoURL == nil {
		return bad("Correction invalide.")
	}
	ctx := r.Context()
	var out mockReview
	err = transaction(ctx, a.db, func(tx pgx.Tx) error {
		uid, eligible, e := a.reviewableMock(ctx, tx, id)
		if e != nil {
			return e
		}
		if !eligible && in.ReviewVideoURL != nil && *in.ReviewVideoURL != "" {
			return problem{403, "La correction vidéo nécessite la formule Suivi."}
		}
		s, e := loadMock(ctx, tx, uid, id, nil, false, eligible)
		if e != nil {
			return e
		}
		pending, e := assessWritten(&s, in.Assessments)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "UPDATE mock_sessions SET results=$2,needs_review=$3 WHERE id=$1", id, s.Results, pending); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO mock_video_reviews(session_id,feedback,review_video_url) VALUES($1,$2,coalesce($3,'')) ON CONFLICT(session_id) DO UPDATE SET feedback=excluded.feedback,review_video_url=coalesce($3,mock_video_reviews.review_video_url)`, id, in.Feedback, in.ReviewVideoURL)
		if e != nil {
			return e
		}
		items, e := a.mockReviews(ctx, tx, "", id)
		if e != nil {
			return e
		}
		if len(items) != 1 {
			return missing()
		}
		out = items[0]
		return nil
	})
	if err != nil {
		return err
	}
	return jsonOut(w, 200, out)
}
