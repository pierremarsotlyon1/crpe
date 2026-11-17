//go:build integration

package learning

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pierremarsotlyon1/crpe/backend/internal/testdatabase"
)

type mailbox struct{ body string }

func (m *mailbox) Send(_ context.Context, _, _, body string) error { m.body = body; return nil }

type browser struct {
	t     *testing.T
	h     http.Handler
	token string
}

func (b browser) req(method, path, body string, want int) map[string]any {
	b.t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if b.token != "" {
		r.Header.Set("Authorization", "Bearer "+b.token)
	}
	w := httptest.NewRecorder()
	b.h.ServeHTTP(w, r)
	if w.Code != want {
		b.t.Fatalf("%s %s returned %d %s (want %d)", method, path, w.Code, w.Body.String(), want)
	}
	var v map[string]any
	if w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
			b.t.Fatal(err)
		}
	}
	return v
}
func TestLearningLifecycleAcrossReplicas(t *testing.T) {
	db := testdatabase.Open(t)
	mail := new(mailbox)
	makeApp := func() http.Handler {
		app, err := New(db, mail, nil, Options{WebURL: "http://localhost:3000", MobileURL: "crpemaths://auth", AdminEmails: []string{"teacher@example.fr"}}, slog.Default())
		if err != nil {
			t.Fatal(err)
		}
		mux := http.NewServeMux()
		app.Register(mux)
		return mux
	}
	one, two := makeApp(), makeApp()
	guest := browser{t: t, h: one}
	guest.req("GET", "/v1/catalog", "", 401)
	login := func(email string) browser {
		guest.req("POST", "/v1/auth/request", fmt.Sprintf(`{"email":%q,"client":"mobile"}`, email), 202)
		_, secret, _ := strings.Cut(mail.body, "#token=")
		verify := fmt.Sprintf(`{"token":%q,"client":"mobile"}`, secret)
		u := browser{t: t, h: two}
		session := u.req("POST", "/v1/auth/verify", verify, 200)
		guest.req("POST", "/v1/auth/verify", verify, 400)
		u.token = session["token"].(string)
		return u
	}
	teacher := login("teacher@example.fr")
	student := login("student@example.fr")
	other := login("other@example.fr")
	student.req("GET", "/v1/admin/content", "", 403)
	student.req("POST", "/v1/admin/content", "{}", 403)
	ch := teacher.req("POST", "/v1/admin/content", `{"kind":"chapter","title":"Test chapter","body":"Fixture"}`, 201)["id"].(string)
	input := fmt.Sprintf(`{"kind":"exercise","parentId":%q,"title":"Fixture","body":"Test prompt","skill":"Fixture","format":"numeric","answer":"2.5","solution":"Reference","tolerance":0.01}`, ch)
	ex := teacher.req("POST", "/v1/admin/content", input, 201)["id"].(string)
	teacher.req("POST", "/v1/admin/content/"+ex+"/validate", "", 409)
	if got := student.req("GET", "/v1/catalog", "", 200); len(got["items"].([]any)) != 0 {
		t.Fatal("draft leaked")
	}
	teacher.req("POST", "/v1/admin/content/"+ch+"/validate", "", 200)
	teacher.req("POST", "/v1/admin/content/"+ex+"/validate", "", 200)
	catalog := student.req("GET", "/v1/catalog", "", 200)["items"].([]any)
	if len(catalog) != 2 {
		t.Fatal(catalog)
	}
	for _, item := range catalog {
		if _, leak := item.(map[string]any)["answer"]; leak {
			t.Fatal("reference leaked")
		}
	}
	aid := "00000000-0000-4000-a000-000000000001"
	answer := fmt.Sprintf(`{"clientId":%q,"exerciseId":%q,"answer":"2,5"}`, aid, ex)
	a := student.req("POST", "/v1/attempts", answer, 200)
	if a["correct"] != true {
		t.Fatal(a)
	}
	student.req("POST", "/v1/attempts", answer, 200)
	student.req("POST", "/v1/attempts", strings.Replace(answer, "2,5", "3,5", 1), 409)
	p := student.req("GET", "/v1/progress", "", 200)
	if p["attempts"] != 1.0 || p["streak"] != 1.0 {
		t.Fatal(p)
	}
	if got := student.req("GET", "/v1/practice?mode=daily", "", 200); len(got["items"].([]any)) != 0 {
		t.Fatal("exercise not postponed")
	}
	var data bytes.Buffer
	form := multipart.NewWriter(&data)
	if err := form.WriteField("exerciseId", ex); err != nil {
		t.Fatal(err)
	}
	if err := form.WriteField("text", "Test answer"); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/v1/submissions", &data)
	r.Header.Set("Content-Type", form.FormDataContentType())
	r.Header.Set("Authorization", "Bearer "+student.token)
	w := httptest.NewRecorder()
	two.ServeHTTP(w, r)
	if w.Code != 201 {
		t.Fatalf("submission %d %s", w.Code, w.Body.String())
	}
	var sub Submission
	if err := json.Unmarshal(w.Body.Bytes(), &sub); err != nil {
		t.Fatal(err)
	}
	if len(other.req("GET", "/v1/submissions", "", 200)["items"].([]any)) != 0 {
		t.Fatal("submission leaked")
	}
	student.req("POST", "/v1/admin/submissions/"+sub.ID+"/review", `{"feedback":"x"}`, 403)
	other.req("GET", "/v1/submissions/"+sub.ID, "", 404)
	checkProgress := func(stage string, attempts float64) {
		t.Helper()
		detail := student.req("GET", "/v1/submissions/"+sub.ID, "", 200)["correctionProgress"].(map[string]any)
		list := student.req("GET", "/v1/submissions", "", 200)["items"].([]any)[0].(map[string]any)["correctionProgress"].(map[string]any)
		if detail["stage"] != stage || detail["attempts"] != attempts || detail["updatedAt"] == nil || list["stage"] != stage {
			t.Fatalf("want %s %v got %v / %v", stage, attempts, detail, list)
		}
	}
	if sub.CorrectionProgress.Stage != "teacher_review" {
		t.Fatal(sub.CorrectionProgress)
	}
	checkProgress("teacher_review", 0)
	if _, err := db.Exec(t.Context(), `INSERT INTO jobs(kind,payload,deduplication_key) VALUES('correct_submission',jsonb_build_object('submissionId',$1::text),'correction/'||$1)`, sub.ID); err != nil {
		t.Fatal(err)
	}
	checkProgress("queued", 0)
	change := func(sql string) {
		t.Helper()
		if _, err := db.Exec(t.Context(), "UPDATE jobs SET "+sql+" WHERE deduplication_key='correction/'||$1", sub.ID); err != nil {
			t.Fatal(err)
		}
	}
	change("status='running',claim_token=gen_random_uuid(),lease_until=now()+interval '1 minute',attempts=1")
	checkProgress("processing", 1)
	change("lease_until=now()-interval '1 second'")
	checkProgress("retrying", 1)
	change("status='pending',claim_token=NULL,lease_until=NULL")
	checkProgress("retrying", 1)
	change("status='failed'")
	checkProgress("failed", 1)
	if _, err := db.Exec(t.Context(), `UPDATE submissions SET correction='{"needsTeacherReview":true}' WHERE id=$1`, sub.ID); err != nil {
		t.Fatal(err)
	}
	checkProgress("teacher_review", 1)
	teacher.req("GET", "/v1/submissions/"+sub.ID, "", 200)
	teacher.req("POST", "/v1/admin/submissions/"+sub.ID+"/review", `{"feedback":"Commentaire"}`, 200)
	checkProgress("completed", 1)
	if student.req("GET", "/v1/submissions", "", 200)["items"].([]any)[0].(map[string]any)["feedback"] != "Commentaire" {
		t.Fatal("missing review")
	}
	teacher.req("PUT", "/v1/admin/content/"+ch, `{"kind":"chapter","title":"Changed","body":"Draft again"}`, 200)
	if len(student.req("GET", "/v1/catalog", "", 200)["items"].([]any)) != 0 {
		t.Fatal("ancestor draft leaked")
	}
	student.req("GET", "/v1/me/export", "", 200)
	student.req("DELETE", "/v1/me", "", 204)
	student.req("GET", "/v1/me", "", 401)
	teacher.req("POST", "/v1/auth/logout", "", 204)
	teacher.req("GET", "/v1/me", "", 401)
}
func TestWaitlistConsentLifecycle(t *testing.T) {
	db := testdatabase.Open(t)
	mail := new(mailbox)
	app, err := New(db, mail, nil, Options{WebURL: "http://localhost:3000", MobileURL: "crpemaths://auth"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	app.Register(mux)
	b := browser{t: t, h: mux}
	b.req("POST", "/v1/waitlist", `{"email":"student@example.fr","consent":false}`, 400)
	b.req("POST", "/v1/waitlist", `{"email":"student@example.fr","consent":true}`, 202)
	lines := strings.Split(mail.body, "#token=")
	confirm := strings.Fields(lines[1])[0]
	leave := strings.Fields(lines[2])[0]
	b.req("POST", "/v1/waitlist/confirm", fmt.Sprintf(`{"token":%q}`, confirm), 204)
	b.req("POST", "/v1/waitlist/confirm", fmt.Sprintf(`{"token":%q}`, confirm), 400)
	b.req("POST", "/v1/waitlist/unsubscribe", fmt.Sprintf(`{"token":%q}`, leave), 204)
	var n int
	if err := db.QueryRow(t.Context(), "SELECT count(*) FROM consents").Scan(&n); err != nil || n != 3 {
		t.Fatal(n, err)
	}
}

func TestMockAndOralWorkflows(t *testing.T) {
	db := testdatabase.Open(t)
	mail := new(mailbox)
	app, err := New(db, mail, nil, Options{WebURL: "http://localhost:3000", MobileURL: "crpemaths://auth", AdminEmails: []string{"teacher@example.fr"}}, slog.Default())
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
	student := login("student@example.fr")
	other := login("other@example.fr")
	for _, x := range []string{"0", "1", "2"} {
		teacher.req("POST", "/v1/admin/content", fmt.Sprintf(`{"kind":"chapter","title":%q,"body":"Fixture"}`, x), 201)
	}
	imp := `{"clientId":"ab5fee68-cf66-442f-9bac-1b2572e40f3e","items":[{"kind":"lesson","title":"Fixture"}]}`
	a := teacher.req("POST", "/v1/admin/import", imp, 200)
	b := teacher.req("POST", "/v1/admin/import", imp, 200)
	if a["items"].([]any)[0].(map[string]any)["id"] != b["items"].([]any)[0].(map[string]any)["id"] {
		t.Fatal("duplicate import")
	}
	subject := teacher.req("POST", "/v1/admin/content", `{"kind":"mock","title":"Mock","body":"Fixture","durationMinutes":60}`, 201)["id"].(string)
	start := fmt.Sprintf(`{"subjectId":%q}`, subject)
	student.req("POST", "/v1/mock-sessions", start, 404)
	teacher.req("POST", "/v1/admin/content/"+subject+"/validate", "", 200)
	item := teacher.req("POST", "/v1/admin/content", fmt.Sprintf(`{"kind":"exercise","parentId":%q,"title":"Fixture","body":"Prompt","skill":"Fixture","format":"numeric","answer":"4","solution":"Reference","points":3}`, subject), 201)["id"].(string)
	student.req("POST", "/v1/mock-sessions", start, 409)
	teacher.req("POST", "/v1/admin/content/"+item+"/validate", "", 200)
	s := student.req("POST", "/v1/mock-sessions", start, 201)
	id := s["id"].(string)
	if _, leak := s["items"].([]any)[0].(map[string]any)["solution"]; leak {
		t.Fatal("solution leaked")
	}
	other.req("GET", "/v1/mock-sessions/"+id, "", 404)
	ans := fmt.Sprintf(`{"answers":[{"exerciseId":%q,"answer":"4"}]}`, item)
	student.req("PUT", "/v1/mock-sessions/"+id+"/answers", ans, 200)
	if _, err := db.Exec(t.Context(), "UPDATE mock_sessions SET deadline=now()-interval '1 second' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	done := student.req("POST", "/v1/mock-sessions/"+id+"/finish", strings.Replace(ans, `"4"`, `"3"`, 1), 200)
	if done["score"] != 3.0 || done["maxScore"] != 3.0 {
		t.Fatal(done)
	}
	oral := teacher.req("POST", "/v1/admin/content", `{"kind":"oral","title":"Oral","body":"Test","criteria":["Critère"],"choices":["Question 1","Question 2"]}`, 201)["id"].(string)
	teacher.req("POST", "/v1/admin/content/"+oral+"/validate", "", 200)
	session := student.req("POST", "/v1/oral-sessions", "{}", 201)
	sid := session["id"].(string)
	student.req("POST", "/v1/oral-sessions/"+sid+"/finish", `{"transcript":"Exposé","answers":["A","B"]}`, 200)
	teacher.req("POST", "/v1/admin/oral-sessions/"+sid+"/review", `{"feedback":[{"criterion":"Critère","status":"validated","comment":"Clair."}]}`, 200)
	items := student.req("GET", "/v1/oral-sessions", "", 200)["items"].([]any)
	if items[0].(map[string]any)["status"] != "reviewed" {
		t.Fatal(items)
	}
	if len(other.req("GET", "/v1/oral-sessions", "", 200)["items"].([]any)) != 0 {
		t.Fatal("oral session leaked")
	}
	student.req("GET", "/v1/revision-plan", "", 200)
}

type testObjects map[string][]byte

func (m testObjects) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	data, err := io.ReadAll(r)
	m[key] = data
	return err
}
func (m testObjects) Get(_ context.Context, key string) (io.ReadCloser, string, error) {
	data, ok := m[key]
	if !ok {
		return nil, "", fmt.Errorf("absent test object")
	}
	return io.NopCloser(bytes.NewReader(data)), "", nil
}
func (m testObjects) Delete(_ context.Context, key string) error { delete(m, key); return nil }
func TestPrivateFilesAndValidatedPublications(t *testing.T) {
	db := testdatabase.Open(t)
	mail := new(mailbox)
	store := testObjects{}
	app, err := New(db, mail, store, Options{WebURL: "http://localhost:3000", MobileURL: "crpemaths://auth", AdminEmails: []string{"teacher@example.fr"}, CorrectionEnabled: true}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	app.Register(mux)
	guest := browser{t: t, h: mux}
	login := func(email string) browser {
		guest.req("POST", "/v1/auth/request", fmt.Sprintf(`{"email":%q,"client":"mobile"}`, email), 202)
		_, s, _ := strings.Cut(mail.body, "#token=")
		session := guest.req("POST", "/v1/auth/verify", fmt.Sprintf(`{"token":%q,"client":"mobile"}`, s), 200)
		return browser{t, mux, session["token"].(string)}
	}
	teacher, student, other := login("teacher@example.fr"), login("student@example.fr"), login("other@example.fr")
	form := func(b browser, path string, fields map[string]string, files map[string][]byte) *httptest.ResponseRecorder {
		t.Helper()
		var body bytes.Buffer
		f := multipart.NewWriter(&body)
		for k, v := range fields {
			if err := f.WriteField(k, v); err != nil {
				t.Fatal(err)
			}
		}
		for k, v := range files {
			p, err := f.CreateFormFile(k, "fixture")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = p.Write(v); err != nil {
				t.Fatal(err)
			}
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest("POST", path, &body)
		r.Header.Set("Content-Type", f.FormDataContentType())
		r.Header.Set("Authorization", "Bearer "+b.token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	read := func(b browser, path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Authorization", "Bearer "+b.token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	mock := teacher.req("POST", "/v1/admin/content", `{"kind":"mock","title":"Fixture","body":"Private text","publicCorrection":"Approved public article","sourceUrl":"https://example.invalid/paper"}`, 201)["id"].(string)
	teacher.req("POST", "/v1/admin/content/"+mock+"/validate", "", 200)
	w := form(teacher, "/v1/admin/content/"+mock+"/documents", map[string]string{"title": "Source PDF"}, map[string][]byte{"file": []byte("%PDF-1.7\nfixture\n%%EOF\n")})
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var doc Document
	if err = json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	path := "/v1/documents/" + doc.ID + "/file"
	if got := read(student, path); got.Code != 404 {
		t.Fatal("draft PDF visible", got.Code)
	}
	if got := read(teacher, path); got.Code != 200 {
		t.Fatal(got.Code)
	}
	teacher.req("POST", "/v1/admin/content/"+mock+"/validate", "", 200)
	if got := read(student, path); got.Code != 200 || got.Header().Get("Content-Type") != "application/pdf" {
		t.Fatal(got.Code)
	}
	if items := guest.req("GET", "/v1/public/subjects", "", 200)["items"].([]any); len(items) != 0 {
		t.Fatal("sales-off published content")
	}
	app.opts.SalesEnabled = true
	items := guest.req("GET", "/v1/public/subjects", "", 200)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["body"] != "Approved public article" || len(items[0].(map[string]any)["documents"].([]any)) != 0 {
		t.Fatal(items)
	}
	app.opts.SalesEnabled = false
	ex := teacher.req("POST", "/v1/admin/content", `{"kind":"exercise","title":"Fixture","body":"Test","skill":"Test","format":"numeric","answer":"2","solution":"Reference"}`, 201)["id"].(string)
	teacher.req("POST", "/v1/admin/content/"+ex+"/validate", "", 200)
	var img bytes.Buffer
	if err := png.Encode(&img, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	fields := map[string]string{"exerciseId": ex, "clientId": "447b8f06-0224-4f79-91f8-000000000001"}
	payload := map[string][]byte{"image": img.Bytes()}
	w = form(student, "/v1/submissions", fields, payload)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var sub Submission
	if err = json.Unmarshal(w.Body.Bytes(), &sub); err != nil {
		t.Fatal(err)
	}
	if sub.CorrectionProgress.Stage != "queued" || !sub.HasImage {
		t.Fatal(sub)
	}
	if second := form(student, "/v1/submissions", fields, payload); second.Code != 201 || !strings.Contains(second.Body.String(), sub.ID) {
		t.Fatal("retry", second.Code, second.Body.String())
	}
	fields["text"] = "changed"
	if got := form(student, "/v1/submissions", fields, payload); got.Code != 409 {
		t.Fatal("conflicting retry", got.Code)
	}
	path = "/v1/submissions/" + sub.ID + "/image"
	if got := read(other, path); got.Code != 404 {
		t.Fatal("image leaked")
	}
	if got := read(student, path); got.Code != 200 || got.Header().Get("Content-Type") != "image/png" {
		t.Fatal(got.Code)
	}
	student.req("DELETE", "/v1/me", "", 204)
	var cleanup int
	if err = db.QueryRow(t.Context(), "SELECT count(*) FROM jobs WHERE kind='delete_object' AND deduplication_key LIKE 'delete/%'").Scan(&cleanup); err != nil || cleanup != 1 {
		t.Fatal("missing cleanup", cleanup, err)
	}
}

func TestWebSessionCookieAndOrigin(t *testing.T) {
	db := testdatabase.Open(t)
	mail := new(mailbox)
	app, err := New(db, mail, nil, Options{WebURL: "https://crpe.example", MobileURL: "crpemaths://auth"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	app.Register(mux)
	b := browser{t: t, h: mux}
	b.req("POST", "/v1/auth/request", `{"email":"member@example.fr"}`, 202)
	if !strings.Contains(mail.body, "https://crpe.example/connexion/confirmer#token=") {
		t.Fatal("invalid link destination")
	}
	_, value, _ := strings.Cut(mail.body, "#token=")
	b.req("POST", "/v1/auth/verify", fmt.Sprintf(`{"client":"mobile","token":%q}`, value), 400)
	r := httptest.NewRequest("POST", "https://crpe.example/v1/auth/verify", strings.NewReader(fmt.Sprintf(`{"token":%q}`, value)))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 200 || strings.Contains(w.Body.String(), `"token"`) {
		t.Fatal("web session exposed bearer credential")
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].Path != "/" || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatal("invalid web cookie")
	}
	r = httptest.NewRequest("DELETE", "https://crpe.example/v1/me", nil)
	r.AddCookie(cookies[0])
	r.Header.Set("Origin", "https://other.example")
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("cross-origin mutation accepted", w.Code)
	}
	r = httptest.NewRequest("GET", "https://crpe.example/v1/me", nil)
	r.AddCookie(cookies[0])
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal("valid session unavailable")
	}
}
