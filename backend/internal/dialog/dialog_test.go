package dialog

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestImageRequestAndUsage(t *testing.T) {
	client := Client{model: "test-model", endpoint: "https://provider.invalid/chat", key: "test-key"}
	client.http = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !json.Valid(raw) || !bytes.Contains(raw, []byte("data:image/png;base64,AQID")) || r.Header.Get("Authorization") != "Bearer test-key" || r.Header.Get("Idempotency-Key") != "copy-1" {
			t.Fatal("invalid provider request")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"model":"test-model-revision","choices":[{"message":{"content":"{\"valid\":true}"}}],"usage":{"prompt_tokens":17,"completion_tokens":4}}`))}, nil
	})}
	var out struct{ Valid bool }
	usage, err := client.CompleteWithImage(t.Context(), "copy-1", "Correct using teacher criteria", map[string]string{"input": "fixture"}, []byte{1, 2, 3}, "image/png", &out)
	if err != nil || !out.Valid || usage.InputTokens != 17 || usage.OutputTokens != 4 || usage.Model != "test-model-revision" {
		t.Fatal(usage, err)
	}
}
