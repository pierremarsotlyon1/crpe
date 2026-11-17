package voice

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestConfiguredAudioEndpoints(t *testing.T) {
	env := map[string]string{"STT_ENDPOINT": "https://voice.invalid/transcribe", "STT_MODEL": "stt-fixture", "TTS_ENDPOINT": "https://voice.invalid/speech", "TTS_MODEL": "tts-fixture", "TTS_VOICE": "test-voice"}
	v, err := FromEnv(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	v.client.Transport = transport(func(r *http.Request) (*http.Response, error) {
		var out string
		switch r.URL.Path {
		case "/transcribe":
			if err := r.ParseMultipartForm(64 << 10); err != nil {
				t.Fatal(err)
			}
			if r.FormValue("model") != "stt-fixture" || r.FormValue("language") != "fr" {
				t.Fatal("invalid transcription form")
			}
			out = `{"text":"Exposé de test."}`
		case "/speech":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "test-voice") {
				t.Fatal("missing voice")
			}
			out = "fixture-mp3-bytes"
		default:
			t.Fatal(r.URL.Path)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(out))}, nil
	})
	s, err := v.Transcribe(t.Context(), []byte("test"), "webm")
	if err != nil || s != "Exposé de test." {
		t.Fatal(s, err)
	}
	b, err := v.Speak(t.Context(), "Question ?")
	if err != nil || len(b) == 0 {
		t.Fatal(err)
	}
}
