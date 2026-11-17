package learning

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

type mockResult struct {
	ExerciseID      string   `json:"exerciseId"`
	Correct         *bool    `json:"correct,omitempty"`
	Feedback        string   `json:"feedback"`
	TeacherFeedback string   `json:"teacherFeedback,omitempty"`
	Points          *float64 `json:"points,omitempty"`
	MaxPoints       float64  `json:"maxPoints"`
	GradingStatus   string   `json:"gradingStatus"`
}
type mockSession struct {
	GradingStatus   string            `json:"gradingStatus,omitempty"`
	AutomaticPoints float64           `json:"automaticPoints"`
	TeacherFeedback string            `json:"teacherFeedback,omitempty"`
	ReviewVideoURL  string            `json:"reviewVideoUrl,omitempty"`
	ID              string            `json:"id"`
	SubjectID       string            `json:"subjectId"`
	StartedAt       time.Time         `json:"startedAt"`
	Deadline        time.Time         `json:"deadline"`
	Status          string            `json:"status"`
	Items           []Content         `json:"items"`
	Results         []mockResult      `json:"results"`
	Score           float64           `json:"score"`
	MaxScore        float64           `json:"maxScore"`
	Answers         map[string]string `json:"answers"`
}
type mockAnswer struct {
	ExerciseID string `json:"exerciseId"`
	Answer     string `json:"answer"`
}

func (a *App) startMock(w http.ResponseWriter, r *http.Request, u User) error {
	var in struct {
		SubjectID string `json:"subjectId"`
	}
	if err := decode(w, r, &in); err != nil {
		return err
	}
	if !uuid(in.SubjectID) {
		return bad("Sujet invalide.")
	}
	ctx := r.Context()
	var session mockSession
	err := transaction(ctx, a.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,77012))", u.ID+in.SubjectID); err != nil {
			return err
		}
		var id string
		err := tx.QueryRow(ctx, "SELECT id::text FROM mock_sessions WHERE user_id=$1 AND subject_id=$2 AND NOT completed", u.ID, in.SubjectID).Scan(&id)
		if err == nil {
			session, err = loadMock(ctx, tx, u.ID, id, nil, false, u.SupportAccess)
			return err
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var minutes int
		if err = tx.QueryRow(ctx, "SELECT duration_minutes FROM published_content WHERE id=$1 AND kind='mock'", in.SubjectID).Scan(&minutes); err != nil {
			return err
		}
		questions, err := contents(ctx, tx, "SELECT "+contentColumns+" FROM published_content c WHERE c.parent_id=$1 AND c.kind='exercise' ORDER BY position,id LIMIT 101", in.SubjectID)
		if err != nil {
			return err
		}
		if len(questions) == 0 || len(questions) > 100 {
			return problem{409, "Ce sujet doit comporter entre 1 et 100 exercices validés."}
		}
		snapshot, err := json.Marshal(questions)
		if err != nil {
			return err
		}
		if err = tx.QueryRow(ctx, `INSERT INTO mock_sessions(user_id,subject_id,deadline,questions) VALUES($1,$2,now()+make_interval(mins=>$3::int),$4) RETURNING id::text`, u.ID, in.SubjectID, minutes, snapshot).Scan(&id); err != nil {
			return err
		}
		session, err = loadMock(ctx, tx, u.ID, id, nil, false, u.SupportAccess)
		return err
	})
	if err != nil {
		return err
	}
	return jsonOut(w, 201, session)
}
func (a *App) mock(w http.ResponseWriter, r *http.Request, u User) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var in struct {
		Answers []mockAnswer `json:"answers"`
	}
	if r.Method != http.MethodGet {
		if err = decode(w, r, &in); err != nil {
			return err
		}
		if len(in.Answers) > 100 {
			return bad("Trop de réponses.")
		}
	}
	var out mockSession
	ctx := r.Context()
	err = transaction(ctx, a.db, func(tx pgx.Tx) error {
		var e error
		out, e = loadMock(ctx, tx, u.ID, id, in.Answers, r.Method == http.MethodPost, u.SupportAccess)
		return e
	})
	if err != nil {
		return err
	}
	return jsonOut(w, 200, out)
}
func loadMock(ctx context.Context, tx pgx.Tx, uid, id string, answers []mockAnswer, finish, includeVideo bool) (mockSession, error) {
	var s mockSession
	var completed, expired bool
	var questions []AdminContent
	err := tx.QueryRow(ctx, `SELECT id::text,subject_id::text,started_at,deadline,questions,answers,completed,deadline<=clock_timestamp(),results FROM mock_sessions WHERE user_id=$1 AND id=$2 FOR UPDATE`, uid, id).Scan(&s.ID, &s.SubjectID, &s.StartedAt, &s.Deadline, &questions, &s.Answers, &completed, &expired, &s.Results)
	if err != nil {
		return s, err
	}
	s.Items = make([]Content, len(questions))
	known := make(map[string]string, len(questions))
	for i, q := range questions {
		s.Items[i] = q.Content
		known[q.ID] = q.Format
		s.MaxScore += q.Points
	}
	if !completed {
		if !expired {
			for _, a := range answers {
				format, ok := known[a.ExerciseID]
				limit := 4096
				if format == "text" {
					limit = 48000
				}
				if !ok || len(a.Answer) > limit {
					return s, bad("Réponse invalide pour ce sujet.")
				}
				s.Answers[a.ExerciseID] = a.Answer
			}
		}
		if expired || finish {
			completed = true
			s.Results = make([]mockResult, 0, len(questions))
			for _, q := range questions {
				result := mockResult{ExerciseID: q.ID, MaxPoints: q.Points, Feedback: q.Solution, GradingStatus: "pending_review"}
				if q.Format != "text" {
					correct, _ := grade(q, s.Answers[q.ID])
					points := 0.0
					if correct {
						points = q.Points
					}
					result.Points = &points
					result.Correct = &correct
					result.GradingStatus = "complete"
				} else {
					s.GradingStatus = "pending_review"
				}
				s.Results = append(s.Results, result)
			}
		}
		if len(answers) > 0 || completed {
			if _, err = tx.Exec(ctx, "UPDATE mock_sessions SET answers=$2,results=$3,completed=$4,needs_review=$5 WHERE id=$1", id, s.Answers, s.Results, completed, s.GradingStatus == "pending_review"); err != nil {
				return s, err
			}
		}
	}
	s.Status = "active"
	if completed {
		s.Status = "completed"
		s.GradingStatus = "complete"
		for i := range s.Results {
			result := &s.Results[i]
			if result.GradingStatus == "" {
				result.GradingStatus = "complete"
			}
			if result.Points != nil {
				s.Score += *result.Points
				if result.Correct != nil {
					s.AutomaticPoints += *result.Points
				}
			}
			if result.GradingStatus == "pending_review" {
				s.GradingStatus = "pending_review"
			}
		}
	}
	if completed {
		err := tx.QueryRow(ctx, "SELECT feedback,CASE WHEN $2::boolean THEN review_video_url ELSE '' END FROM mock_video_reviews WHERE session_id=$1", id, includeVideo).Scan(&s.TeacherFeedback, &s.ReviewVideoURL)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return s, err
		}
	}
	return s, nil
}
func (a *App) revisionPlan(w http.ResponseWriter, r *http.Request, u User) error {
	// The last three confirmed responses determine mastery. Introduce at most three
	// non-mastered skills per week and put the lowest recent accuracy first.
	ctx := r.Context()
	rows, err := a.db.Query(ctx, `WITH events AS (SELECT skill,correct,row_number() OVER(PARTITION BY skill ORDER BY performed_at DESC,client_id DESC) AS n FROM attempts WHERE user_id=$1),
 recent AS (SELECT skill,count(*) AS n,avg(correct::int) AS accuracy FROM events WHERE n<=3 GROUP BY skill),
 skills AS (SELECT c.skill,(array_agg(c.id::text ORDER BY c.position,c.id))[1:5] AS exercises,coalesce(r.accuracy,0) AS accuracy
 FROM published_content c LEFT JOIN recent r ON c.skill=r.skill WHERE `+practiceFilter+` AND NOT (coalesce(r.n,0)>=3 AND coalesce(r.accuracy,0)=1)
 GROUP BY c.skill,r.accuracy), ranked AS (SELECT *,((row_number() OVER(ORDER BY accuracy,skill)-1)/3+1)::int AS week FROM skills)
 SELECT week,skill,exercises FROM ranked ORDER BY week,accuracy,skill`, u.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type week struct {
		Week        int      `json:"week"`
		Skills      []string `json:"skills"`
		ExerciseIDs []string `json:"exerciseIds"`
	}
	weeks := []week{}
	for rows.Next() {
		var n int
		var skill string
		var ids []string
		if err := rows.Scan(&n, &skill, &ids); err != nil {
			return err
		}
		if n > len(weeks) {
			weeks = append(weeks, week{Week: n})
		}
		weeks[n-1].Skills = append(weeks[n-1].Skills, skill)
		weeks[n-1].ExerciseIDs = append(weeks[n-1].ExerciseIDs, ids...)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	return jsonOut(w, 200, map[string]any{"weeks": weeks})
}
