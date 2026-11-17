package learning

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pierremarsotlyon1/crpe/backend/internal/grading"
)

type AttemptInput struct {
	ClientID    string     `json:"clientId"`
	ExerciseID  string     `json:"exerciseId"`
	Answer      string     `json:"answer"`
	PerformedAt *time.Time `json:"performedAt,omitempty"`
}
type Attempt struct {
	ClientID     string    `json:"clientId"`
	ExerciseID   string    `json:"exerciseId"`
	Correct      bool      `json:"correct"`
	Feedback     string    `json:"feedback"`
	NextReviewAt time.Time `json:"nextReviewAt"`
}
type SkillProgress struct {
	Skill        string    `json:"skill"`
	Attempts     int       `json:"attempts"`
	Correct      int       `json:"correct"`
	NextReviewAt time.Time `json:"nextReviewAt"`
}

func grade(c AdminContent, answer string) (bool, error) {
	switch c.Format {
	case "numeric":
		correct, known := grading.Numeric(c.Answer, answer, c.Tolerance)
		if !known {
			return false, bad("Saisissez une réponse numérique.")
		}
		return correct, nil
	case "choice":
		if !slices.Contains(c.Choices, answer) {
			return false, bad("Sélectionnez un choix proposé.")
		}
		return answer == c.Answer, nil
	default:
		return false, bad("Déposez cette réponse dans l’espace des copies.")
	}
}
func reviewDelay(streak int) time.Duration {
	days := [...]int{1, 3, 7, 14, 30, 60}
	return time.Duration(days[min(max(0, streak), len(days)-1)]) * 24 * time.Hour
}
func (a *App) attempt(w http.ResponseWriter, r *http.Request, u User) error {
	var in AttemptInput
	if err := decode(w, r, &in); err != nil {
		return err
	}
	if !uuid(in.ExerciseID) || !uuid(in.ClientID) || len(in.Answer) > 4096 {
		return bad("Tentative invalide.")
	}
	in.Answer = strings.TrimSpace(in.Answer)
	in.ClientID = strings.ToLower(in.ClientID)
	in.ExerciseID = strings.ToLower(in.ExerciseID)
	ctx := r.Context()
	now := time.Now().UTC()
	at := now
	if in.PerformedAt != nil {
		at = in.PerformedAt.UTC()
	}
	var out Attempt
	err := transaction(ctx, a.db, func(tx pgx.Tx) error {
		// Serialize a learner's concurrent sync requests and make retries return the same grade.
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,8419))", u.ID); err != nil {
			return err
		}
		var answer string
		var original time.Time
		err := tx.QueryRow(ctx, "SELECT client_id::text,exercise_id::text,correct,feedback,next_review_at,answer,performed_at FROM attempts WHERE user_id=$1 AND client_id=$2", u.ID, in.ClientID).Scan(&out.ClientID, &out.ExerciseID, &out.Correct, &out.Feedback, &out.NextReviewAt, &answer, &original)
		if err == nil {
			if out.ExerciseID != in.ExerciseID || answer != in.Answer || in.PerformedAt != nil && !original.Equal(at.Truncate(time.Microsecond)) {
				return problem{409, "Identifiant déjà utilisé pour une autre réponse."}
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if at.Before(now.Add(-90*24*time.Hour)) || at.After(now.Add(5*time.Minute)) {
			return bad("Date de tentative hors plage.")
		}
		c, err := scanContent(tx.QueryRow(ctx, "SELECT "+contentColumns+` FROM published_content c WHERE c.id=$1 AND kind='exercise' AND NOT EXISTS (SELECT FROM content p WHERE p.id=c.parent_id AND p.kind='mock')`, in.ExerciseID))
		if err != nil {
			return err
		}
		correct, err := grade(c, in.Answer)
		if err != nil {
			return err
		}
		out = Attempt{ClientID: in.ClientID, ExerciseID: in.ExerciseID, Correct: correct, Feedback: c.Solution}
		// The number of consecutive successful answers at the event time sets its next review.
		var streak int
		if correct {
			err = tx.QueryRow(ctx, `SELECT count(*) FROM (SELECT correct,bool_or(NOT correct) OVER(ORDER BY performed_at DESC,client_id DESC) AS stopped FROM attempts
 WHERE user_id=$1 AND exercise_id=$2 AND (performed_at,client_id)<($3,$4::uuid)) s WHERE NOT stopped`, u.ID, in.ExerciseID, at, in.ClientID).Scan(&streak)
			if err != nil {
				return err
			}
			streak++
		}
		out.NextReviewAt = at.Add(reviewDelay(streak)).Truncate(time.Microsecond)
		_, err = tx.Exec(ctx, `INSERT INTO attempts(user_id,client_id,exercise_id,answer,correct,feedback,skill,performed_at,next_review_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, u.ID, in.ClientID, in.ExerciseID, in.Answer, correct, c.Solution, c.Skill, at, out.NextReviewAt)
		if err != nil {
			return err
		}
		// Offline insertion can change subsequent streaks. Recalculate their due dates from event order.
		_, err = tx.Exec(ctx, `WITH grouped AS (SELECT client_id,performed_at,correct,sum((NOT correct)::int) OVER(ORDER BY performed_at,client_id) AS grp
 FROM attempts WHERE user_id=$1 AND exercise_id=$2), lengths AS (SELECT client_id,performed_at,count(*) FILTER(WHERE correct) OVER(PARTITION BY grp ORDER BY performed_at,client_id) AS n FROM grouped)
 UPDATE attempts a SET next_review_at=s.performed_at+make_interval(days=>(ARRAY[1,3,7,14,30,60])[least(s.n+1,6)::int]) FROM lengths s WHERE a.user_id=$1 AND a.client_id=s.client_id AND (s.performed_at,s.client_id)>($3,$4::uuid)`, u.ID, in.ExerciseID, at, in.ClientID)
		return err
	})
	if err != nil {
		return err
	}
	return jsonOut(w, 200, out)
}

const practiceFilter = `c.exam_id='crpe' AND c.kind='exercise' AND c.format IN ('numeric','choice') AND NOT EXISTS (SELECT FROM content p WHERE p.id=c.parent_id AND p.kind='mock')`

func (a *App) practice(w http.ResponseWriter, r *http.Request, u User) error {
	mode := r.URL.Query().Get("mode")
	var query string
	switch mode {
	case "", "daily", "revision":
		limit := "5"
		if mode == "revision" {
			limit = "20"
		}
		query = "SELECT " + contentColumns + ` FROM published_content c LEFT JOIN LATERAL (SELECT correct,next_review_at FROM attempts WHERE user_id=$1 AND exercise_id=c.id ORDER BY performed_at DESC,client_id DESC LIMIT 1) a ON TRUE WHERE ` + practiceFilter + ` AND (a.next_review_at IS NULL OR a.next_review_at<=now()) ORDER BY a.correct ASC NULLS LAST,a.next_review_at,c.position,c.id LIMIT ` + limit
	case "diagnostic":
		query = `SELECT ` + contentColumns + ` FROM (SELECT c.*,row_number() OVER(PARTITION BY c.skill ORDER BY c.position,c.id) AS skill_rank FROM published_content c WHERE ` + practiceFilter + `) c ORDER BY c.skill_rank,c.skill,c.position,c.id LIMIT 20`
	default:
		return bad("Mode d’entraînement inconnu.")
	}
	var args []any
	if mode != "diagnostic" {
		args = []any{u.ID}
	}
	items, err := contents(r.Context(), a.db, query, args...)
	if err != nil {
		return err
	}
	return publicList(w, items)
}
func (a *App) progress(w http.ResponseWriter, r *http.Request, u User) error {
	ctx := r.Context()
	rows, err := a.db.Query(ctx, `WITH last_review AS (SELECT DISTINCT ON(exercise_id) exercise_id,skill,next_review_at FROM attempts WHERE user_id=$1 ORDER BY exercise_id,performed_at DESC,client_id DESC),
 due AS (SELECT skill,min(next_review_at) AS next_due FROM last_review GROUP BY skill)
 SELECT a.skill,count(*),count(*) FILTER(WHERE a.correct),d.next_due FROM attempts a JOIN due d ON a.skill=d.skill WHERE a.user_id=$1 GROUP BY a.skill,d.next_due ORDER BY a.skill`, u.ID)
	if err != nil {
		return err
	}
	skills, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (SkillProgress, error) {
		var s SkillProgress
		err := row.Scan(&s.Skill, &s.Attempts, &s.Correct, &s.NextReviewAt)
		return s, err
	})
	if err != nil {
		return err
	}
	var total, correct, streak int
	for _, s := range skills {
		total += s.Attempts
		correct += s.Correct
	}
	err = a.db.QueryRow(ctx, `WITH dates AS (SELECT DISTINCT (performed_at AT TIME ZONE 'Europe/Paris')::date AS d FROM attempts WHERE user_id=$1 AND performed_at<=now()+interval '5 minutes'), numbered AS (SELECT d,d+row_number() OVER(ORDER BY d DESC)::int AS grp FROM dates)
 SELECT count(*) FROM numbered WHERE grp=(SELECT grp FROM numbered WHERE d>=(now() AT TIME ZONE 'Europe/Paris')::date-1 ORDER BY d DESC LIMIT 1)`, u.ID).Scan(&streak)
	if err != nil {
		return err
	}
	return jsonOut(w, 200, map[string]any{"attempts": total, "correct": correct, "streak": streak, "skills": skills})
}
