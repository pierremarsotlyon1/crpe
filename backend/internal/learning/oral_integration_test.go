//go:build integration

package learning

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pierremarsotlyon1/crpe/backend/internal/dialog"
	"github.com/pierremarsotlyon1/crpe/backend/internal/queue"
	"github.com/pierremarsotlyon1/crpe/backend/internal/testdatabase"
	"github.com/pierremarsotlyon1/crpe/backend/internal/voice"
)

func TestOralHTTPProvidersAndDurableReview(t *testing.T) {
	db := testdatabase.Open(t)
	ctx := t.Context()
	mail := new(mailbox)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/transcribe":
			if err := r.ParseMultipartForm(1 << 16); err != nil {
				t.Error(err)
				http.Error(w, "bad", 400)
				return
			}
			if r.FormValue("language") != "fr" || r.FormValue("model") != "fixture-stt" {
				t.Error("invalid provider transcription fields")
			}
			if _, err := fmt.Fprint(w, `{"text":"Je justifie ma réponse."}`); err != nil {
				t.Error(err)
			}
		case "/speech":
			var request map[string]string
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			if request["input"] != "Question A ?" || request["voice"] != "fixture-voice" {
				t.Error(request)
			}
			if _, err := w.Write([]byte("fixture-audio")); err != nil {
				t.Error(err)
			}
		case "/chat":
			var in map[string]any
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				t.Error(err)
			}
			if len(in["messages"].([]any)) != 2 {
				t.Error(in)
			}
			output := `{"confident":true,"feedback":[{"criterion":"Argumentation","status":"validated","comment":"La démarche est explicitée.","evidence":"Je justifie"}]}`
			if err := json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": output}}}}); err != nil {
				t.Error(err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()
	env := map[string]string{"STT_ENDPOINT": provider.URL + "/transcribe", "STT_MODEL": "fixture-stt", "TTS_ENDPOINT": provider.URL + "/speech", "TTS_MODEL": "fixture-tts", "TTS_VOICE": "fixture-voice", "LLM_ENDPOINT": provider.URL + "/chat", "LLM_MODEL": "fixture-llm"}
	get := func(k string) string { return env[k] }
	speaker, err := voice.FromEnv(get)
	if err != nil {
		t.Fatal(err)
	}
	model, err := dialog.FromEnv(get)
	if err != nil {
		t.Fatal(err)
	}
	app, err := New(db, mail, nil, Options{WebURL: "http://localhost:3000", MobileURL: "crpemaths://auth", AdminEmails: []string{"teacher@example.fr"}, Voice: speaker, Dialog: model}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	app.Register(mux)
	anon := browser{t: t, h: mux}
	login := func(email string) browser {
		anon.req("POST", "/v1/auth/request", fmt.Sprintf(`{"email":%q,"client":"mobile"}`, email), 202)
		_, key, _ := strings.Cut(mail.body, "#token=")
		s := anon.req("POST", "/v1/auth/verify", fmt.Sprintf(`{"token":%q,"client":"mobile"}`, key), 200)
		return browser{t, mux, s["token"].(string)}
	}
	teacher, student := login("teacher@example.fr"), login("student@example.fr")
	oral := teacher.req("POST", "/v1/admin/content", `{"kind":"oral","title":"Oral test","body":"Énoncé","criteria":["Argumentation"],"choices":["Question A ?","Question B ?"]}`, 201)["id"].(string)
	teacher.req("POST", "/v1/admin/content/"+oral+"/validate", "", 200)
	id := student.req("POST", "/v1/oral-sessions", "{}", 201)["id"].(string)
	request := func(path, ct string, body io.Reader) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/v1/oral-sessions/"+id+path, body)
		r.Header.Set("Content-Type", ct)
		r.Header.Set("Authorization", "Bearer "+student.token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	speech := request("/speech", "application/json", strings.NewReader(`{"index":0}`))
	if speech.Code != 200 || speech.Header().Get("Content-Type") != "audio/mpeg" || speech.Body.String() != "fixture-audio" {
		t.Fatal(speech.Code, speech.Body.String())
	}
	var data bytes.Buffer
	f := multipart.NewWriter(&data)
	part, err := f.CreateFormFile("audio", "sample.webm")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write([]byte{0x1a, 0x45, 0xdf, 0xa3, 0, 1}); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	transcription := request("/transcribe", f.FormDataContentType(), &data)
	if transcription.Code != 200 || !strings.Contains(transcription.Body.String(), "Je justifie") {
		t.Fatal(transcription.Code, transcription.Body.String())
	}
	student.req("POST", "/v1/oral-sessions/"+id+"/finish", `{"transcript":"Je justifie ma réponse.","answers":["Première réponse.","Seconde réponse."]}`, 200)
	delivery, err := queue.New(db).Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if delivery.Message().Kind != "review_oral" {
		t.Fatal(delivery.Message().Kind)
	}
	handler := OralReviewHandler(db, model)
	if err = handler(ctx, delivery.Message()); err != nil {
		t.Fatal(err)
	}
	if err = delivery.Ack(ctx); err != nil {
		t.Fatal(err)
	}
	if err = handler(ctx, delivery.Message()); err != nil {
		t.Fatal(err)
	}
	got := student.req("GET", "/v1/oral-sessions", "", 200)["items"].([]any)[0].(map[string]any)
	if got["feedbackSource"] != "ai" || got["status"] != "reviewed" || got["feedback"].([]any)[0].(map[string]any)["status"] != "validated" {
		t.Fatal(got)
	}
	teacher.req("GET", "/v1/admin/stats", "", 200)
}
