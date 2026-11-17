package correction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pierremarsotlyon1/crpe/backend/internal/dialog"
	"github.com/pierremarsotlyon1/crpe/backend/internal/objectstore"
	"github.com/pierremarsotlyon1/crpe/backend/internal/worker"
)

func Handler(db *pgxpool.Pool, store objectstore.Store, model Model, opts Options, logger *slog.Logger) worker.Handler {
	return func(ctx context.Context, msg worker.Message) error {
		var in struct {
			ID string `json:"submissionId"`
		}
		if err := json.Unmarshal(msg.Payload, &in); err != nil || len(in.ID) != 36 {
			return errors.New("invalid correction job")
		}
		allowed, err := reserve(ctx, db, in.ID, opts)
		if err != nil {
			return err
		}
		return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
			var text, key, mime, owner string
			var e exercise
			err := tx.QueryRow(ctx, `SELECT s.text_answer,s.object_key,s.content_type,s.user_id::text,c.format,c.answer,c.solution,c.body,c.skill,c.tolerance,c.criteria FROM submissions s JOIN published_content c ON c.id=s.exercise_id WHERE s.id=$1 AND s.correction IS NULL AND s.status='pending_review' FOR UPDATE OF s`, in.ID).Scan(&text, &key, &mime, &owner, &e.Format, &e.Answer, &e.Solution, &e.Body, &e.Skill, &e.Tolerance, &e.Criteria)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			if err != nil {
				return err
			}
			r := localResult(e.Format, e.Answer, text, e.Solution, e.Tolerance)
			usage := dialog.Usage{}
			outcome := "completed"
			if !allowed {
				r = localResult("text", e.Answer, text, e.Solution, e.Tolerance)
				r.Summary = "Le quota quotidien de correction automatique est atteint. Votre copie est transmise à l’enseignante."
				outcome = "quota"
			} else if opts.Mode == "provider" && model != nil {
				var data []byte
				if key != "" {
					if store == nil {
						return errors.New("image storage unavailable")
					}
					body, _, err := store.Get(ctx, key)
					if err != nil {
						return err
					}
					data, err = io.ReadAll(io.LimitReader(body, (8<<20)+1))
					closeErr := body.Close()
					if err != nil {
						return err
					}
					if closeErr != nil {
						return closeErr
					}
					if len(data) > 8<<20 {
						return errors.New("stored image exceeds correction limit")
					}
				}
				var providerErr error
				r, usage, providerErr = evaluate(ctx, model, in.ID, e, text, data, mime)
				if providerErr != nil {
					// A provider outage must not leave a learner without a route to review.
					// Log no copy, email, prompt, token or provider response body.
					outcome = "provider_unavailable"
					logger.WarnContext(ctx, "automatic correction routed to teacher", "submission_id", in.ID)
				}
			} else if opts.Mode != "local" || key != "" {
				r = localResult("text", e.Answer, text, e.Solution, e.Tolerance)
				outcome = "teacher_review"
			}
			if r.NeedsTeacherReview && outcome == "completed" {
				outcome = "teacher_review"
			}
			var similar string
			err = tx.QueryRow(ctx, `SELECT c.id::text FROM published_content c WHERE c.kind='exercise' AND $2<>'' AND c.skill=$2 AND NOT EXISTS(SELECT 1 FROM content parent WHERE parent.id=c.parent_id AND parent.kind='mock') AND c.id<>(SELECT exercise_id FROM submissions WHERE id=$1) ORDER BY c.position,c.id LIMIT 1`, in.ID, e.Skill).Scan(&similar)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			r.SimilarExerciseID = similar
			status := "ai_reviewed"
			if r.NeedsTeacherReview {
				status = "pending_review"
			}
			encoded, err := json.Marshal(r)
			if err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `UPDATE submissions SET correction=$2,status=$3 WHERE id=$1`, in.ID, encoded, status); err != nil {
				return err
			}
			var cost *float64
			if opts.InputCost != nil && opts.OutputCost != nil && usage.Model != "" && (usage.InputTokens > 0 || usage.OutputTokens > 0) {
				amount := (float64(usage.InputTokens)*(*opts.InputCost) + float64(usage.OutputTokens)*(*opts.OutputCost)) / 1_000_000
				cost = &amount
			}
			_, err = tx.Exec(ctx, `UPDATE correction_usage SET finished_at=now(),model=$2,input_tokens=$3,output_tokens=$4,estimated_cost_usd=$5,outcome=$6 WHERE submission_id=$1`, in.ID, usage.Model, usage.InputTokens, usage.OutputTokens, cost, outcome)
			return err
		})
	}
}

// Reserve a quota slot durably before sending anything to a paid provider.
// A duplicate delivery reuses that reservation, including after a process crash.
func reserve(ctx context.Context, db *pgxpool.Pool, id string, o Options) (bool, error) {
	allowed := false
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		var user string
		err := tx.QueryRow(ctx, `SELECT user_id::text FROM submissions WHERE id=$1`, id).Scan(&user)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,13))`, user); err != nil {
			return err
		}
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM correction_usage WHERE submission_id=$1)`, id).Scan(&exists); err != nil {
			return err
		}
		if exists {
			allowed = true
			return nil
		}
		var count int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM correction_usage WHERE user_id=$1 AND reserved_at>=date_trunc('day',now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'`, user).Scan(&count); err != nil {
			return err
		}
		if count >= o.DailyLimit {
			return nil
		}
		_, err = tx.Exec(ctx, `INSERT INTO correction_usage(submission_id,user_id,provider) VALUES($1,$2,$3)`, id, user, o.Mode)
		if err != nil {
			return fmt.Errorf("reserve correction quota: %w", err)
		}
		allowed = true
		return nil
	})
	return allowed, err
}
