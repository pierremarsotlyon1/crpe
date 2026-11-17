//go:build integration

package correction

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"testing"

	"github.com/pierremarsotlyon1/crpe/backend/internal/testdatabase"
	"github.com/pierremarsotlyon1/crpe/backend/internal/worker"
)

func TestConcurrentCorrectionsShareQuotaAndReplaySafely(t *testing.T) {
	db := testdatabase.Open(t)
	ctx := t.Context()
	var user, ex string
	if err := db.QueryRow(ctx, `INSERT INTO users(email,role) VALUES('fixture@example.test','student') RETURNING id::text`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO content(exam_id,kind,title,body,format,answer,solution,skill,status) VALUES('crpe','exercise','Isolated fixture','1+1','numeric','2','1+1=2','fixture','validated') RETURNING id::text`).Scan(&ex); err != nil {
		t.Fatal(err)
	}
	jobs := make([]worker.Message, 12)
	for i := range jobs {
		var id string
		if err := db.QueryRow(ctx, `INSERT INTO submissions(user_id,exercise_id,text_answer) VALUES($1,$2,'2') RETURNING id::text`, user, ex).Scan(&id); err != nil {
			t.Fatal(err)
		}
		jobs[i] = worker.Message{ID: fmt.Sprint(i), Payload: json.RawMessage(fmt.Sprintf(`{"submissionId":%q}`, id))}
	}
	h := Handler(db, nil, nil, Options{Mode: "local", DailyLimit: 2}, slog.Default())
	var wg sync.WaitGroup
	for _, job := range jobs {
		wg.Go(func() {
			if err := h(ctx, job); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	var charged, completed, teacher int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM correction_usage`).Scan(&charged); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FILTER(WHERE status='ai_reviewed'),count(*) FILTER(WHERE status='pending_review') FROM submissions`).Scan(&completed, &teacher); err != nil {
		t.Fatal(err)
	}
	if charged != 2 || completed != 2 || teacher != 10 {
		t.Fatalf("charged=%d completed=%d teacher=%d", charged, completed, teacher)
	}
	for _, job := range jobs {
		if err := h(ctx, job); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM correction_usage`).Scan(&charged); err != nil || charged != 2 {
		t.Fatalf("replay quota=%d %v", charged, err)
	}
	// A teacher's final review is never overwritten by a stale delivery.
	if _, err := db.Exec(ctx, `UPDATE submissions SET status='reviewed',feedback='Teacher final',correction=NULL`); err != nil {
		t.Fatal(err)
	}
	for _, job := range jobs {
		if err := h(ctx, job); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM submissions WHERE status='reviewed' AND feedback='Teacher final' AND correction IS NULL`).Scan(&teacher); err != nil || teacher != 12 {
		t.Fatalf("teacher overwritten: %d %v", teacher, err)
	}
	// Withdrawing a reference while its correction is queued prevents disclosure.
	if _, err := db.Exec(ctx, `UPDATE content SET status='draft' WHERE id=$1`, ex); err != nil {
		t.Fatal(err)
	}
	var withdrawn string
	if err := db.QueryRow(ctx, `INSERT INTO submissions(user_id,exercise_id,text_answer) VALUES($1,$2,'2') RETURNING id::text`, user, ex).Scan(&withdrawn); err != nil {
		t.Fatal(err)
	}
	if err := h(ctx, worker.Message{Payload: json.RawMessage(fmt.Sprintf(`{"submissionId":%q}`, withdrawn))}); err != nil {
		t.Fatal(err)
	}
	var hidden bool
	if err := db.QueryRow(ctx, `SELECT correction IS NULL AND status='pending_review' FROM submissions WHERE id=$1`, withdrawn).Scan(&hidden); err != nil || !hidden {
		t.Fatal("withdrawn reference leaked", err)
	}
	// A deleted account leaves neither usage nor a resurrectable correction.
	if _, err := db.Exec(ctx, `DELETE FROM users WHERE id=$1`, user); err != nil {
		t.Fatal(err)
	}
	if err := h(ctx, jobs[0]); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM correction_usage`).Scan(&charged); err != nil || charged != 0 {
		t.Fatal(charged, err)
	}
}

func TestProviderUsageAndFallbackArePersisted(t *testing.T) {
	db := testdatabase.Open(t)
	ctx := t.Context()
	var id string
	err := db.QueryRow(ctx, `WITH u AS(INSERT INTO users(email,role) VALUES('provider-fixture@example.test','student') RETURNING id), c AS(INSERT INTO content(exam_id,kind,title,format,answer,solution,status) VALUES('crpe','exercise','Isolated fixture','numeric','2','2','validated') RETURNING id) INSERT INTO submissions(user_id,exercise_id,text_answer) SELECT u.id,c.id,'3' FROM u,c RETURNING id::text`).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	rate := 1.0
	h := Handler(db, nil, fixtureModel{response: `{"confident":true,"verdict":"correct","summary":"Wrong claim","errors":[],"writingFeedback":[]}`}, Options{Mode: "provider", DailyLimit: 20, InputCost: &rate, OutputCost: &rate}, slog.Default())
	if err = h(ctx, worker.Message{Payload: json.RawMessage(fmt.Sprintf(`{"submissionId":%q}`, id))}); err != nil {
		t.Fatal(err)
	}
	var result Result
	var raw []byte
	var status string
	var cost float64
	if err = db.QueryRow(ctx, `SELECT s.correction,s.status,u.estimated_cost_usd::float8 FROM submissions s JOIN correction_usage u ON u.submission_id=s.id WHERE s.id=$1`, id).Scan(&raw, &status, &cost); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result.Verdict != "uncertain" || result.GeneratedByAI || !result.NeedsTeacherReview || status != "pending_review" || cost != .00008 {
		t.Fatalf("result=%+v status=%s cost=%v", result, status, cost)
	}
}
