package learning

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type Voice interface {
	Transcribe(context.Context, []byte, string) (string, error)
	Speak(context.Context, string) ([]byte, error)
}
type CriterionFeedback struct {
	Criterion string `json:"criterion"`
	Status    string `json:"status"`
	Comment   string `json:"comment"`
}
type oralSubject struct {
	Title           string   `json:"title"`
	Body            string   `json:"body"`
	Criteria        []string `json:"criteria"`
	Questions       []string `json:"questions"`
	PrepMinutes     int      `json:"prepMinutes"`
	DurationMinutes int      `json:"durationMinutes"`
}
type oralSession struct {
	FeedbackSource string `json:"feedbackSource"`
	oralSubject
	ID         string              `json:"id"`
	SubjectID  string              `json:"subjectId"`
	CreatedAt  time.Time           `json:"createdAt"`
	Status     string              `json:"status"`
	Transcript string              `json:"transcript"`
	Answers    []string            `json:"answers"`
	Feedback   []CriterionFeedback `json:"feedback"`
}

const oralColumns = `id::text,subject_id::text,snapshot,created_at,status,transcript,answers,feedback,feedback_source`

func scanOral(r pgx.Row) (oralSession, error) {
	var s oralSession
	err := r.Scan(&s.ID, &s.SubjectID, &s.oralSubject, &s.CreatedAt, &s.Status, &s.Transcript, &s.Answers, &s.Feedback, &s.FeedbackSource)
	return s, err
}
func (a *App) startOral(w http.ResponseWriter, r *http.Request, u User) error {
	var in struct {
		SubjectID string `json:"subjectId"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	if in.SubjectID != "" && !uuid(in.SubjectID) {
		return bad("Sujet invalide.")
	}
	ctx := r.Context()
	if err := a.rate(ctx, "oral-session/"+u.ID, 12); err != nil {
		return err
	}
	s, err := scanOral(a.db.QueryRow(ctx, `INSERT INTO oral_sessions(user_id,subject_id,snapshot)
 SELECT $1,id,jsonb_build_object('title',title,'body',body,'criteria',criteria,'questions',choices,'prepMinutes',prep_minutes,'durationMinutes',duration_minutes)
 FROM published_content WHERE kind='oral' AND (nullif($2,'')::uuid IS NULL OR id=nullif($2,'')::uuid) ORDER BY random() LIMIT 1 RETURNING `+oralColumns, u.ID, in.SubjectID))
	if err != nil {
		return err
	}
	return jsonOut(w, 201, s)
}
func (a *App) listOral(w http.ResponseWriter, r *http.Request, u User) error {
	admin := strings.HasPrefix(r.URL.Path, "/v1/admin/")
	rows, err := a.db.Query(r.Context(), "SELECT "+oralColumns+" FROM oral_sessions WHERE user_id=$1 OR ($2::boolean AND status<>'active') ORDER BY created_at DESC", u.ID, admin)
	if err != nil {
		return err
	}
	items, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (oralSession, error) { return scanOral(r) })
	if err != nil {
		return err
	}
	return jsonOut(w, 200, map[string]any{"items": items})
}
func (a *App) finishOral(w http.ResponseWriter, r *http.Request, u User) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var in struct {
		Transcript string   `json:"transcript"`
		Answers    []string `json:"answers"`
	}
	if err = decode(w, r, &in); err != nil {
		return err
	}
	in.Transcript = strings.TrimSpace(in.Transcript)
	if in.Transcript == "" || len(in.Transcript) > 120000 || len(in.Answers) < 2 || len(in.Answers) > 3 {
		return bad("Exposé et réponses requis.")
	}
	for _, v := range in.Answers {
		if strings.TrimSpace(v) == "" || len(v) > 16000 {
			return bad("Réponse orale invalide.")
		}
	}
	ctx := r.Context()
	var s oralSession
	err = transaction(ctx, a.db, func(tx pgx.Tx) error {
		var e error
		s, e = scanOral(tx.QueryRow(ctx, `UPDATE oral_sessions SET transcript=$3,answers=$4,status='pending_review' WHERE id=$1 AND user_id=$2 AND status='active' AND jsonb_array_length(snapshot->'questions')=cardinality($4::text[]) RETURNING `+oralColumns, id, u.ID, in.Transcript, in.Answers))
		if e != nil {
			return e
		}
		if a.opts.Dialog != nil {
			_, e = tx.Exec(ctx, "INSERT INTO jobs(kind,payload,deduplication_key) VALUES('review_oral',jsonb_build_object('id',$1::text),'oral/'||$1)", id)
		}
		return e
	})
	if err != nil {
		return err
	}
	return jsonOut(w, 200, s)
}
func (a *App) reviewOral(w http.ResponseWriter, r *http.Request, _ User) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var in struct {
		Feedback []CriterionFeedback `json:"feedback"`
	}
	if err = decode(w, r, &in); err != nil {
		return err
	}
	ctx := r.Context()
	var out oralSession
	err = transaction(ctx, a.db, func(tx pgx.Tx) error {
		s, e := scanOral(tx.QueryRow(ctx, "SELECT "+oralColumns+" FROM oral_sessions WHERE id=$1 AND status<>'active' FOR UPDATE", id))
		if e != nil {
			return e
		}
		if len(in.Feedback) != len(s.Criteria) {
			return bad("Évaluez chaque critère de la grille.")
		}
		for i, c := range in.Feedback {
			if c.Criterion != s.Criteria[i] || c.Status != "validated" && c.Status != "to_work" || len(c.Comment) > 4096 {
				return bad("Évaluation de critère invalide.")
			}
		}
		out, e = scanOral(tx.QueryRow(ctx, "UPDATE oral_sessions SET feedback=$2,status='reviewed',feedback_source='teacher' WHERE id=$1 RETURNING "+oralColumns, id, in.Feedback))
		return e
	})
	if err != nil {
		return err
	}
	return jsonOut(w, 200, out)
}
func (a *App) activeOral(r *http.Request, u User) (oralSession, error) {
	id, err := pathID(r)
	if err != nil {
		return oralSession{}, err
	}
	return scanOral(a.db.QueryRow(r.Context(), "SELECT "+oralColumns+" FROM oral_sessions WHERE id=$1 AND user_id=$2 AND status='active'", id, u.ID))
}
func audioExt(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte{0x1a, 0x45, 0xdf, 0xa3}):
		return "webm"
	case bytes.HasPrefix(data, []byte("OggS")):
		return "ogg"
	case len(data) > 12 && string(data[4:8]) == "ftyp":
		return "mp4"
	case len(data) > 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WAVE":
		return "wav"
	default:
		return ""
	}
}
func (a *App) transcribe(w http.ResponseWriter, r *http.Request, u User) error {
	if _, err := a.activeOral(r, u); err != nil {
		return err
	}
	if a.opts.Voice == nil {
		return unavailable()
	}
	if err := a.rate(r.Context(), "voice/"+u.ID, 20); err != nil {
		return err
	}
	f, err := multipartFields(w, r, map[string]int{"audio": 20 << 20}, 20<<20)
	if err != nil {
		return err
	}
	data := f["audio"]
	ext := audioExt(data)
	if ext == "" {
		return problem{415, "Enregistrement WebM, Ogg, MP4 ou WAV attendu."}
	}
	transcript, err := a.opts.Voice.Transcribe(r.Context(), data, ext)
	if err != nil {
		return unavailable()
	}
	return jsonOut(w, 200, map[string]string{"text": transcript})
}
func (a *App) speak(w http.ResponseWriter, r *http.Request, u User) error {
	s, err := a.activeOral(r, u)
	if err != nil {
		return err
	}
	if a.opts.Voice == nil {
		return unavailable()
	}
	var in struct {
		Index *int `json:"index"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	if in.Index == nil || *in.Index < 0 || *in.Index >= len(s.Questions) {
		return bad("Question inconnue.")
	}
	if err = a.rate(r.Context(), "speech/"+u.ID, 24); err != nil {
		return err
	}
	data, err := a.opts.Voice.Speak(r.Context(), s.Questions[*in.Index])
	if err != nil {
		return unavailable()
	}
	w.Header().Set("Content-Type", "audio/mpeg")
	_, err = w.Write(data)
	return err
}
