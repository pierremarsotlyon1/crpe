//go:build integration

package learning

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/pierremarsotlyon1/crpe/backend/internal/billing"
	"github.com/pierremarsotlyon1/crpe/backend/internal/testdatabase"
)

type bankSelection struct{ ids []string }

func (m *bankSelection) Complete(_ context.Context, _, _ string, _ any, out any) error {
	data, err := json.Marshal(map[string]any{"exerciseIds": m.ids})
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}
func TestSuiviAndWrittenMockReview(t *testing.T) {
	db := testdatabase.Open(t)
	mail := new(mailbox)
	model := new(bankSelection)
	env := map[string]string{"BILLING_PLANS_JSON": `[{"key":"standard","name":"Autonomie","priceId":"price_std"},{"key":"coached","name":"Suivi","priceId":"price_coach","tier":"suivi"}]`}
	commerce, err := billing.New(db, func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	app, err := New(db, mail, nil, Options{WebURL: "https://prep.example", MobileURL: "crpemaths://auth", AdminEmails: []string{"teacher@example.fr"}, Billing: commerce, Dialog: model}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	app.Register(mux)
	anon := browser{t: t, h: mux}
	login := func(email string) browser {
		anon.req("POST", "/v1/auth/request", fmt.Sprintf(`{"email":%q,"client":"mobile"}`, email), 202)
		_, s, _ := strings.Cut(mail.body, "#token=")
		session := anon.req("POST", "/v1/auth/verify", fmt.Sprintf(`{"token":%q,"client":"mobile"}`, s), 200)
		return browser{t, mux, session["token"].(string)}
	}
	teacher := login("teacher@example.fr")
	student := login("first@example.fr")
	other := login("second@example.fr")
	anon.req("GET", "/v1/support", "", 401)
	other.req("GET", "/v1/support", "", 403)
	if a := student.req("GET", "/v1/me/access", "", 200); a["tier"] != "autonomie" || a["upgradeUrl"] != nil {
		t.Fatal(a)
	}
	teacher.req("PUT", "/v1/admin/support", `{"liveUrl":"https://teacher.example/weekly","liveSchedule":"Mercredi 18 h (Paris)","groupUrl":"https://chat.whatsapp.com/group-test"}`, 200)
	teacher.req("PUT", "/v1/admin/support", `{"liveUrl":"","liveSchedule":"","groupUrl":"https://example.fr/fake"}`, 400)
	makeContent := func(data map[string]any) string {
		raw, err := json.Marshal(data)
		if err != nil {
			t.Fatal(err)
		}
		item := teacher.req("POST", "/v1/admin/content", string(raw), 201)
		id := item["id"].(string)
		teacher.req("POST", "/v1/admin/content/"+id+"/validate", "", 200)
		return id
	}
	subject := makeContent(map[string]any{"kind": "mock", "title": "Annale de test", "body": "Énoncé validé", "sourceTitle": "Archive enseignante"})
	objective := makeContent(map[string]any{"kind": "exercise", "parentId": subject, "title": "Calcul", "body": "Test", "skill": "Nombre", "format": "numeric", "answer": "2", "solution": "Référence", "points": 2})
	written := makeContent(map[string]any{"kind": "exercise", "parentId": subject, "title": "Preuve", "body": "Justifier.", "skill": "Raisonnement", "format": "text", "criteria": []string{"Démonstration"}, "solution": "Démonstration de référence", "points": 4})
	if _, err = db.Exec(t.Context(), "INSERT INTO content_documents(content_id,title,size,object_key) VALUES($1,'Annale de test',16,'documents/fixture')", subject); err != nil {
		t.Fatal(err)
	}
	source := fmt.Sprintf(`{"title":"Assemblage","topic":"Révision","durationMinutes":25,"count":2,"sourceExerciseIds":[%q,%q]}`, objective, written)
	model.ids = []string{"9f424292-dfb5-4544-987f-058b91ad3ca9", written}
	teacher.req("POST", "/v1/admin/mock-drafts", source, 422)
	if n := len(teacher.req("GET", "/v1/admin/content", "", 200)["items"].([]any)); n != 3 {
		t.Fatal("partial bank draft")
	}
	model.ids = []string{written, written}
	teacher.req("POST", "/v1/admin/mock-drafts", source, 422)
	model.ids = []string{written, objective}
	clones := teacher.req("POST", "/v1/admin/mock-drafts", source, 201)["items"].([]any)
	if len(clones) != 3 {
		t.Fatal(clones)
	}
	first := clones[1].(map[string]any)
	if first["format"] != "text" || first["solution"] != "Démonstration de référence" || first["sourceTitle"] != "Archive enseignante" || len(first["documents"].([]any)) != 1 {
		t.Fatal("source snapshot lost", first)
	}
	for _, v := range clones {
		if v.(map[string]any)["status"] != "draft" {
			t.Fatal("auto publication")
		}
	}
	student.req("POST", "/v1/mock-sessions", fmt.Sprintf(`{"subjectId":%q}`, clones[0].(map[string]any)["id"]), 404)
	sid := student.req("POST", "/v1/mock-sessions", fmt.Sprintf(`{"subjectId":%q}`, subject), 201)["id"].(string)
	finish := fmt.Sprintf(`{"answers":[{"exerciseId":%q,"answer":"2"},{"exerciseId":%q,"answer":"Je justifie."}]}`, objective, written)
	got := student.req("POST", "/v1/mock-sessions/"+sid+"/finish", finish, 200)
	if got["gradingStatus"] != "pending_review" || got["automaticPoints"] != 2.0 || got["score"] != 2.0 || got["maxScore"] != 6.0 {
		t.Fatal(got)
	}
	for _, item := range got["results"].([]any) {
		v := item.(map[string]any)
		if v["exerciseId"] == written && (v["points"] != nil || v["correct"] != nil || v["feedback"] != "Démonstration de référence") {
			t.Fatal(v)
		}
	}
	other.req("GET", "/v1/mock-sessions/"+sid, "", 404)
	student.req("POST", "/v1/admin/mock-sessions/"+sid+"/review", "{}", 403)
	if items := teacher.req("GET", "/v1/admin/mock-sessions", "", 200)["items"].([]any); len(items) != 1 {
		t.Fatal(items)
	}
	gradeURL := "/v1/admin/mock-sessions/" + sid + "/review"
	teacher.req("POST", gradeURL, fmt.Sprintf(`{"feedback":"","assessments":[{"exerciseId":%q,"points":5,"feedback":"Test"}]}`, written), 400)
	teacher.req("POST", gradeURL, `{"feedback":"Texte","reviewVideoUrl":"https://video.example/one"}`, 403)
	graded := teacher.req("POST", gradeURL, fmt.Sprintf(`{"feedback":"Avis global","assessments":[{"exerciseId":%q,"points":3,"feedback":"Raisonnement correct."}]}`, written), 200)
	if graded["gradingStatus"] != "complete" {
		t.Fatal(graded)
	}
	got = student.req("GET", "/v1/mock-sessions/"+sid, "", 200)
	if got["score"] != 5.0 || got["automaticPoints"] != 2.0 || got["gradingStatus"] != "complete" || got["teacherFeedback"] != "Avis global" {
		t.Fatal(got)
	}
	if _, err = db.Exec(t.Context(), "INSERT INTO entitlements(user_id,plan_key,source,valid_until) SELECT id,'coached','test',now()+interval '1 day' FROM users WHERE email='first@example.fr'"); err != nil {
		t.Fatal(err)
	}
	if got = student.req("GET", "/v1/me/access", "", 200); got["tier"] != "suivi" {
		t.Fatal(got)
	}
	teacher.req("POST", gradeURL, `{"feedback":"Vidéo personnelle","reviewVideoUrl":"https://video.example/one"}`, 200)
	got = student.req("GET", "/v1/support", "", 200)
	if got["groupUrl"] != "https://chat.whatsapp.com/group-test" || got["mockReviews"].([]any)[0].(map[string]any)["reviewVideoUrl"] != "https://video.example/one" {
		t.Fatal(got)
	}
	if _, err = db.Exec(t.Context(), "UPDATE entitlements SET valid_until=now()-interval '1 day'"); err != nil {
		t.Fatal(err)
	}
	student.req("GET", "/v1/support", "", 403)
	got = student.req("GET", "/v1/mock-sessions/"+sid, "", 200)
	if got["reviewVideoUrl"] != nil {
		t.Fatal("expired video leaked", got)
	}
	if got := student.req("GET", "/v1/mock-sessions", "", 200)["items"].([]any)[0].(map[string]any); got["reviewVideoUrl"] != "" {
		t.Fatal("history video leaked")
	}
	personal := student.req("GET", "/v1/me/export", "", 200)
	raw, _ := json.Marshal(personal)
	if !strings.Contains(string(raw), "https://video.example/one") {
		t.Fatal("owner export missing personal review")
	}
	raw, _ = json.Marshal(other.req("GET", "/v1/me/export", "", 200))
	if strings.Contains(string(raw), "https://video.example/one") {
		t.Fatal("foreign review leaked through export")
	}
	env["SALES_ENABLED"] = "true"
	commerce, err = billing.New(db, func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	app.opts.Billing = commerce
	app.opts.SalesEnabled = true
	got = student.req("GET", "/v1/me/access", "", 200)
	if got["tier"] != "none" || !strings.HasPrefix(got["upgradeUrl"].(string), "https://prep.example/offres?") {
		t.Fatal(got)
	}
	student.req("GET", "/v1/catalog", "", 403)
}
