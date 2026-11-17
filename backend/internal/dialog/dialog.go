// Package dialog sends structured text requests to a configurable chat endpoint.
package dialog

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	endpoint, model, key string
	http                 *http.Client
}

func FromEnv(get func(string) string) (*Client, error) {
	ep, model, key := get("LLM_ENDPOINT"), get("LLM_MODEL"), get("LLM_API_KEY")
	if ep == "" && model == "" && key == "" {
		return nil, nil
	}
	u, err := url.Parse(ep)
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || model == "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || net.ParseIP(u.Hostname()).IsLoopback()))) {
		return nil, errors.New("LLM_ENDPOINT requires HTTPS or loopback and LLM_MODEL")
	}
	return &Client{ep, model, key, &http.Client{Timeout: 35 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

type Usage struct {
	InputTokens  int
	OutputTokens int
	Model        string
}

func (c *Client) Complete(ctx context.Context, key, system string, input any, out any) error {
	_, err := c.CompleteWithImage(ctx, key, system, input, nil, "", out)
	return err
}
func (c *Client) CompleteWithImage(ctx context.Context, key, system string, input any, image []byte, mime string, out any) (Usage, error) {
	var usage Usage
	if c == nil {
		return usage, errors.New("dialog provider unavailable")
	}
	usage.Model = c.model
	text, err := json.Marshal(input)
	if err != nil {
		return usage, err
	}
	var content any = string(text)
	if len(image) > 0 {
		if len(image) > 8<<20 || (mime != "image/jpeg" && mime != "image/png") {
			return usage, errors.New("invalid dialog image")
		}
		content = []map[string]any{{"type": "text", "text": string(text)}, {"type": "image_url", "image_url": map[string]string{"url": "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(image)}}}
	}
	body, err := json.Marshal(map[string]any{"model": c.model, "messages": []map[string]any{{"role": "system", "content": system}, {"role": "user", "content": content}}, "response_format": map[string]string{"type": "json_object"}})
	if err != nil {
		return usage, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytes.NewReader(body))
	if err != nil {
		return usage, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return usage, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return usage, errors.New("dialog provider unavailable")
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 256<<10+1))
	if err != nil || len(raw) > 256<<10 {
		return usage, errors.New("invalid dialog response size")
	}
	var result struct {
		Model   string
		Choices []struct{ Message struct{ Content string } }
		Usage   struct {
			Prompt     int `json:"prompt_tokens"`
			Completion int `json:"completion_tokens"`
		}
	}
	if err := json.Unmarshal(raw, &result); err != nil || len(result.Choices) != 1 {
		return usage, errors.New("invalid dialog response")
	}
	usage.InputTokens = max(0, result.Usage.Prompt)
	usage.OutputTokens = max(0, result.Usage.Completion)
	if result.Model != "" {
		usage.Model = result.Model
	}
	d := json.NewDecoder(bytes.NewBufferString(result.Choices[0].Message.Content))
	d.DisallowUnknownFields()
	if err = d.Decode(out); err != nil {
		return usage, errors.New("invalid structured dialog output")
	}
	if err = d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return usage, errors.New("additional dialog response data")
	}
	return usage, nil
}
