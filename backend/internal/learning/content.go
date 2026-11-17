package learning

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/pierremarsotlyon1/crpe/backend/internal/grading"
)

type ContentFields struct {
	VideoDescription string   `json:"videoDescription"`
	DurationMinutes  int      `json:"durationMinutes"`
	PrepMinutes      int      `json:"prepMinutes"`
	Points           float64  `json:"points"`
	ExamID           string   `json:"examId"`
	Kind             string   `json:"kind"`
	ParentID         *string  `json:"parentId,omitempty"`
	Title            string   `json:"title"`
	Body             string   `json:"body"`
	Skill            string   `json:"skill"`
	Position         int      `json:"position"`
	Format           string   `json:"format"`
	Choices          []string `json:"choices"`
	Criteria         []string `json:"criteria"`
	SourceTitle      string   `json:"sourceTitle"`
	SourceURL        string   `json:"sourceUrl"`
	SourceYear       int      `json:"sourceYear"`
	VideoURL         string   `json:"videoUrl"`
}
type Reference struct {
	PublicCorrection string  `json:"publicCorrection"`
	Answer           string  `json:"answer"`
	Solution         string  `json:"solution"`
	Tolerance        float64 `json:"tolerance"`
}
type Content struct {
	ContentFields
	Documents []Document `json:"documents"`
	ID        string     `json:"id"`
	Status    string     `json:"status"`
}
type AdminContent struct {
	Demo bool `json:"demo"`
	Content
	Reference
}
type ContentInput struct {
	ContentFields
	Reference
}

const contentColumns = `c.id::text,c.exam_id,c.kind,c.parent_id::text,c.title,c.body,c.skill,c.position,c.format,c.choices,c.criteria,c.video_url,c.status,c.answer,c.solution,c.tolerance,c.source_title,c.source_url,c.source_year,c.duration_minutes,c.prep_minutes,c.points,c.video_description,c.public_correction,c.demo,COALESCE((SELECT jsonb_agg(jsonb_build_object('id',d.id,'title',d.title,'size',d.size) ORDER BY d.title,d.id) FROM content_documents d WHERE d.content_id=c.id),'[]')`

func scanContent(row pgx.Row) (AdminContent, error) {
	var c AdminContent
	err := row.Scan(&c.ID, &c.ExamID, &c.Kind, &c.ParentID, &c.Title, &c.Body, &c.Skill, &c.Position, &c.Format, &c.Choices, &c.Criteria, &c.VideoURL, &c.Status, &c.Answer, &c.Solution, &c.Tolerance, &c.SourceTitle, &c.SourceURL, &c.SourceYear, &c.DurationMinutes, &c.PrepMinutes, &c.Points, &c.VideoDescription, &c.PublicCorrection, &c.Demo, &c.Documents)
	return c, err
}
func (c *ContentInput) normalize() error {
	if c.DurationMinutes == 0 {
		c.DurationMinutes = 15
		if c.Kind == "mock" {
			c.DurationMinutes = 180
		}
	}
	if c.PrepMinutes == 0 {
		c.PrepMinutes = 10
	}
	if c.Points == 0 {
		c.Points = 1
	}
	if c.DurationMinutes < 1 || c.DurationMinutes > 480 || c.PrepMinutes < 1 || c.PrepMinutes > 120 || c.Points < 0.01 || c.Points > 1000 || !finite(c.Points) {
		return bad("Durée ou barème invalide.")
	}
	if len(c.VideoDescription) > 16000 || len(c.PublicCorrection) > 120000 || c.PublicCorrection != "" && c.Kind != "mock" {
		return bad("Contenu éditorial invalide.")
	}
	if c.ExamID == "" {
		c.ExamID = "crpe"
	}
	if c.Format == "" {
		c.Format = "text"
	}
	if c.Choices == nil {
		c.Choices = []string{}
	}
	if c.Criteria == nil {
		c.Criteria = []string{}
	}
	c.Title = strings.TrimSpace(c.Title)
	c.Skill = strings.TrimSpace(c.Skill)
	c.Answer = strings.TrimSpace(c.Answer)
	if c.Title == "" || len(c.Title) > 300 || len(c.Body) > 120000 || len(c.Skill) > 200 || len(c.Solution) > 32000 || len(c.Answer) > 4096 || c.Position < 0 || c.Position > 1_000_000 || c.Tolerance < 0 || !finite(c.Tolerance) {
		return bad("Champs du contenu invalides.")
	}
	if !slices.Contains([]string{"chapter", "lesson", "exercise", "mock", "oral"}, c.Kind) || !slices.Contains([]string{"numeric", "choice", "text"}, c.Format) {
		return bad("Type de contenu inconnu.")
	}
	if c.ParentID != nil && !uuid(*c.ParentID) {
		return bad("Parent invalide.")
	}
	if c.ExamID != "crpe" {
		return bad("Examen inconnu.")
	}
	if len(c.Criteria) > 50 || len(c.Choices) > 25 {
		return bad("Trop de critères ou de choix.")
	}
	for _, list := range [][]string{c.Criteria, c.Choices} {
		seen := make(map[string]bool)
		for _, v := range list {
			if v != strings.TrimSpace(v) || v == "" || len(v) > 2048 || seen[v] {
				return bad("Chaque choix ou critère doit être unique et renseigné.")
			}
			seen[v] = true
		}
	}
	if len(c.SourceTitle) > 300 || c.SourceYear != 0 && (c.SourceYear < 1900 || c.SourceYear > 2200) {
		return bad("Source invalide.")
	}
	if c.SourceURL != "" {
		u, err := url.Parse(c.SourceURL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || len(c.SourceURL) > 2048 {
			return bad("URL de source HTTPS attendue.")
		}
	}
	if c.VideoURL != "" {
		u, err := url.Parse(c.VideoURL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || len(c.VideoURL) > 2048 {
			return bad("Une URL de vidéo HTTPS est requise.")
		}
	}
	return nil
}
func finite(n float64) bool { return !math.IsNaN(n) && !math.IsInf(n, 0) }

func canPublish(c AdminContent) error {
	if c.Demo {
		return bad("Un chapitre de démonstration demeure un brouillon.")
	}
	if strings.TrimSpace(c.Body) == "" {
		return bad("Ajoutez le contenu avant validation.")
	}
	if c.Kind == "oral" && (len(c.Criteria) == 0 || len(c.Choices) < 2 || len(c.Choices) > 3) {
		return bad("Le sujet oral nécessite des critères et deux ou trois questions.")
	}
	if c.Kind != "exercise" {
		return nil
	}
	if c.Skill == "" || strings.TrimSpace(c.Solution) == "" {
		return bad("Renseignez la notion et le corrigé de référence.")
	}
	switch c.Format {
	case "choice":
		if len(c.Choices) < 2 || !slices.Contains(c.Choices, c.Answer) {
			return bad("Le corrigé doit correspondre à un des choix.")
		}
	case "numeric":
		if _, ok := grading.Number(c.Answer); !ok {
			return bad("Renseignez une réponse numérique finie.")
		}
	case "text":
		if len(c.Criteria) == 0 {
			return bad("Renseignez les critères de correction.")
		}
	}
	return nil
}

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func contents(ctx context.Context, q queryer, sql string, args ...any) ([]AdminContent, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (AdminContent, error) { return scanContent(r) })
}
func publicList(w http.ResponseWriter, items []AdminContent) error {
	out := make([]Content, len(items))
	for i, v := range items {
		out[i] = v.Content
	}
	return jsonOut(w, 200, map[string]any{"items": out})
}
func (a *App) catalog(w http.ResponseWriter, r *http.Request, _ User) error {
	items, err := contents(r.Context(), a.db, "SELECT "+contentColumns+" FROM published_content c WHERE exam_id='crpe' ORDER BY position,title,id")
	if err != nil {
		return err
	}
	return publicList(w, items)
}
func (a *App) publicSubjects(w http.ResponseWriter, r *http.Request, _ User) error {
	var items []AdminContent
	var err error
	if a.opts.SalesEnabled {
		items, err = contents(r.Context(), a.db, "SELECT "+contentColumns+" FROM published_content c WHERE kind='mock' AND btrim(public_correction)<>'' AND exam_id='crpe' ORDER BY position,title,id")
	}
	if err != nil {
		return err
	}
	for i, item := range items {
		items[i].Content = Content{ID: item.ID, Status: item.Status, Documents: []Document{}, ContentFields: ContentFields{ExamID: item.ExamID, Kind: item.Kind, Title: item.Title, Body: item.PublicCorrection, Format: "text", Choices: []string{}, Criteria: []string{}, SourceTitle: item.SourceTitle, SourceURL: item.SourceURL, SourceYear: item.SourceYear}}
	}
	return publicList(w, items)
}
func (a *App) adminCatalog(w http.ResponseWriter, r *http.Request, _ User) error {
	items, err := contents(r.Context(), a.db, "SELECT "+contentColumns+" FROM content c WHERE NOT archived ORDER BY position,title,id")
	if err != nil {
		return err
	}
	return jsonOut(w, 200, map[string]any{"items": items})
}
func contentLock(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(17041832)")
	return err
}
func save(ctx context.Context, tx pgx.Tx, id string, in ContentInput) (AdminContent, error) {
	var out AdminContent
	if err := in.normalize(); err != nil {
		return out, err
	}
	if in.ParentID != nil {
		var parentKind string
		err := tx.QueryRow(ctx, "SELECT kind FROM content WHERE id=$1 AND exam_id=$2 AND NOT archived", *in.ParentID, in.ExamID).Scan(&parentKind)
		if errors.Is(err, pgx.ErrNoRows) {
			return out, bad("Parent inconnu.")
		}
		if err != nil {
			return out, err
		}
		if !(in.Kind == "lesson" && parentKind == "chapter" || in.Kind == "exercise" && slices.Contains([]string{"lesson", "chapter", "mock"}, parentKind)) || id == *in.ParentID {
			return out, bad("Hiérarchie de contenu invalide.")
		}
	}
	args := []any{in.ExamID, in.Kind, in.ParentID, in.Title, in.Body, in.Skill, in.Position, in.Format, in.Choices, in.Criteria, in.VideoURL, in.Answer, in.Solution, in.Tolerance, in.SourceTitle, in.SourceURL, in.SourceYear, in.DurationMinutes, in.PrepMinutes, in.Points, in.VideoDescription, in.PublicCorrection}
	q := `INSERT INTO content AS c(exam_id,kind,parent_id,title,body,skill,position,format,choices,criteria,video_url,answer,solution,tolerance,source_title,source_url,source_year,duration_minutes,prep_minutes,points,video_description,public_correction) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)`
	if id != "" {
		q = `UPDATE content AS c SET exam_id=$1,kind=$2,parent_id=$3,title=$4,body=$5,skill=$6,position=$7,format=$8,choices=$9,criteria=$10,video_url=$11,answer=$12,solution=$13,tolerance=$14,source_title=$15,source_url=$16,source_year=$17,duration_minutes=$18,prep_minutes=$19,points=$20,video_description=$21,public_correction=$22,status='draft'
 WHERE id=$23 AND NOT archived AND kind=$2`
		args = append(args, id)
	}
	return scanContent(tx.QueryRow(ctx, q+" RETURNING "+contentColumns, args...))
}
func (a *App) saveContent(w http.ResponseWriter, r *http.Request, _ User) error {
	var in ContentInput
	if err := decode(w, r, &in); err != nil {
		return err
	}
	id := ""
	code := 201
	if r.Method == http.MethodPut {
		var err error
		id, err = pathID(r)
		if err != nil {
			return err
		}
		code = 200
	}
	var out AdminContent
	ctx := r.Context()
	err := transaction(ctx, a.db, func(tx pgx.Tx) error {
		if err := contentLock(ctx, tx); err != nil {
			return err
		}
		var e error
		out, e = save(ctx, tx, id, in)
		return e
	})
	if err != nil {
		return err
	}
	return jsonOut(w, code, out)
}
func (a *App) publish(w http.ResponseWriter, r *http.Request, _ User) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	var out AdminContent
	err = transaction(ctx, a.db, func(tx pgx.Tx) error {
		if err := contentLock(ctx, tx); err != nil {
			return err
		}
		var e error
		out, e = scanContent(tx.QueryRow(ctx, "SELECT "+contentColumns+" FROM content c WHERE id=$1 AND NOT archived FOR UPDATE", id))
		if e != nil {
			return e
		}
		if e = canPublish(out); e != nil {
			return e
		}
		if out.ParentID != nil {
			var valid bool
			if e = tx.QueryRow(ctx, "SELECT EXISTS (SELECT FROM published_content WHERE id=$1)", *out.ParentID).Scan(&valid); e != nil {
				return e
			}
			if !valid {
				return problem{409, "Validez d’abord le contenu parent."}
			}
		}
		_, e = tx.Exec(ctx, "UPDATE content SET status='validated' WHERE id=$1", id)
		out.Status = "validated"
		return e
	})
	if err != nil {
		return err
	}
	return jsonOut(w, 200, out)
}
func (a *App) archive(w http.ResponseWriter, r *http.Request, _ User) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	err = transaction(ctx, a.db, func(tx pgx.Tx) error {
		if err := contentLock(ctx, tx); err != nil {
			return err
		}
		tag, e := tx.Exec(ctx, `WITH RECURSIVE tree(id) AS (SELECT id FROM content WHERE id=$1 AND NOT archived UNION ALL SELECT c.id FROM content c JOIN tree t ON t.id=c.parent_id) UPDATE content SET archived=true,status='draft' WHERE id IN (SELECT id FROM tree)`, id)
		if e == nil && tag.RowsAffected() == 0 {
			return missing()
		}
		return e
	})
	if err != nil {
		return err
	}
	return empty(w, 204)
}
func (a *App) stats(w http.ResponseWriter, r *http.Request, _ User) error {
	var stats struct {
		Users            int      `json:"users"`
		ConfirmedEmails  int      `json:"confirmedEmails"`
		Attempts         int      `json:"attempts"`
		PendingReviews   int      `json:"pendingReviews"`
		ActiveAccess     int      `json:"activeAccess"`
		AIRuns           int      `json:"aiRuns"`
		AIInputTokens    int64    `json:"aiInputTokens"`
		AIOutputTokens   int64    `json:"aiOutputTokens"`
		AIUnpricedRuns   int      `json:"aiUnpricedRuns"`
		EstimatedCostUSD *float64 `json:"estimatedCostUsd"`
	}
	err := a.db.QueryRow(r.Context(), `SELECT (SELECT count(*) FROM users),
(SELECT count(*) FROM waitlist WHERE confirmed_at IS NOT NULL),(SELECT count(*) FROM attempts),
(SELECT count(*) FROM submission_tracking t WHERE t.correction_progress->>'stage' IN('teacher_review','failed')),
(SELECT count(DISTINCT user_id) FROM member_rights WHERE valid_until>now() AND NOT refunded),
count(*),coalesce(sum(input_tokens),0),coalesce(sum(output_tokens),0),count(*) FILTER(WHERE estimated_cost_usd IS NULL),
CASE WHEN count(*) FILTER(WHERE estimated_cost_usd IS NULL)>0 THEN NULL ELSE coalesce(sum(estimated_cost_usd),0) END
FROM correction_usage WHERE provider='provider' AND finished_at IS NOT NULL`).Scan(&stats.Users, &stats.ConfirmedEmails, &stats.Attempts, &stats.PendingReviews, &stats.ActiveAccess, &stats.AIRuns, &stats.AIInputTokens, &stats.AIOutputTokens, &stats.AIUnpricedRuns, &stats.EstimatedCostUSD)
	if err != nil {
		return err
	}
	return jsonOut(w, 200, stats)
}
func (a *App) importContent(w http.ResponseWriter, r *http.Request, u User) error {
	var in struct {
		ClientID string         `json:"clientId"`
		Items    []ContentInput `json:"items"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	if !uuid(in.ClientID) || len(in.Items) < 1 || len(in.Items) > 100 {
		return bad("Un lot doit contenir 1 à 100 contenus.")
	}
	ctx := r.Context()
	encoded, err := json.Marshal(in.Items)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(encoded)
	var result json.RawMessage
	err = transaction(ctx, a.db, func(tx pgx.Tx) error {
		if err := contentLock(ctx, tx); err != nil {
			return err
		}
		var match bool
		err := tx.QueryRow(ctx, "SELECT request_hash=$3,response FROM content_imports WHERE user_id=$1 AND client_id=$2", u.ID, in.ClientID, digest[:]).Scan(&match, &result)
		if err == nil {
			if !match {
				return problem{409, "Identifiant déjà utilisé pour un autre import."}
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		out := make([]AdminContent, 0, len(in.Items))
		for _, item := range in.Items {
			c, err := save(ctx, tx, "", item)
			if err != nil {
				return err
			}
			out = append(out, c)
		}
		result, err = json.Marshal(map[string]any{"items": out})
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "INSERT INTO content_imports VALUES($1,$2,$3,$4)", u.ID, in.ClientID, digest[:], result)
		return err
	})
	if err != nil {
		return err
	}
	return jsonOut(w, 200, result)
}
