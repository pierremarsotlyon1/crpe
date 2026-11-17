// Package voice connects configurable transcription and speech endpoints.
package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Remote struct {
	client                                                                                  *http.Client
	transcribeURL, transcribeModel, transcribeKey, speechURL, speechModel, speechKey, voice string
}

func FromEnv(get func(string) string) (*Remote, error) {
	r := &Remote{transcribeURL: get("STT_ENDPOINT"), transcribeModel: get("STT_MODEL"), transcribeKey: get("STT_API_KEY"), speechURL: get("TTS_ENDPOINT"), speechModel: get("TTS_MODEL"), speechKey: get("TTS_API_KEY"), voice: get("TTS_VOICE")}
	for _, cfg := range []struct{ url, model, key string }{{r.transcribeURL, r.transcribeModel, r.transcribeKey}, {r.speechURL, r.speechModel, r.speechKey}} {
		if cfg.url == "" && cfg.model == "" && cfg.key == "" {
			continue
		}
		u, err := url.Parse(cfg.url)
		if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || net.ParseIP(u.Hostname()).IsLoopback()))) || cfg.model == "" {
			return nil, errors.New("voice endpoints require HTTPS or loopback, and a configured model")
		}
	}
	if r.speechURL != "" && r.voice == "" {
		return nil, errors.New("TTS_VOICE required")
	}
	r.client = &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return r, nil
}
func (r *Remote) call(ctx context.Context, endpoint, key, mime string, body []byte, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mime)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("voice provider returned %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || int64(len(data)) > limit || len(data) == 0 {
		return nil, errors.New("invalid voice response")
	}
	return data, nil
}
func (r *Remote) Transcribe(ctx context.Context, data []byte, ext string) (string, error) {
	if r == nil || r.transcribeURL == "" {
		return "", errors.New("transcription unavailable")
	}
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	if err := w.WriteField("model", r.transcribeModel); err != nil {
		return "", err
	}
	if err := w.WriteField("language", "fr"); err != nil {
		return "", err
	}
	part, err := w.CreateFormFile("file", "recording."+ext)
	if err != nil {
		return "", err
	}
	if _, err = part.Write(data); err != nil {
		return "", err
	}
	if err = w.Close(); err != nil {
		return "", err
	}
	raw, err := r.call(ctx, r.transcribeURL, r.transcribeKey, w.FormDataContentType(), b.Bytes(), 200_000)
	if err != nil {
		return "", err
	}
	var out struct{ Text string }
	if err := json.Unmarshal(raw, &out); err != nil || strings.TrimSpace(out.Text) == "" {
		return "", errors.New("invalid transcription response")
	}
	return out.Text, nil
}
func (r *Remote) Speak(ctx context.Context, text string) ([]byte, error) {
	if r == nil || r.speechURL == "" {
		return nil, errors.New("speech unavailable")
	}
	body, err := json.Marshal(map[string]string{"model": r.speechModel, "voice": r.voice, "input": text, "response_format": "mp3"})
	if err != nil {
		return nil, err
	}
	return r.call(ctx, r.speechURL, r.speechKey, "application/json", body, 8<<20)
}
