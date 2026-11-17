package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthAndSalesFlag(t *testing.T) {
	dependencyErr := errors.New("password=never-expose-this")
	handler := New(func(ctx context.Context) error {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("readiness dependency has no deadline")
		}
		return dependencyErr
	}, false)
	for _, tt := range []struct {
		path string
		code int
		body string
	}{
		{path: "/health/live", code: 200, body: `{"status":"ok"}`},
		{path: "/health/ready", code: 503, body: `{"status":"unavailable"}`},
		{path: "/v1/config", code: 200, body: `{"salesEnabled":false}`},
	} {
		t.Run(tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if rec.Code != tt.code || strings.TrimSpace(rec.Body.String()) != tt.body {
				t.Fatalf("response: %d %s", rec.Code, rec.Body.String())
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Error("configuration and health responses must not be cached")
			}
		})
	}
}

func TestReadinessAndEnabledFlag(t *testing.T) {
	handler := New(func(context.Context) error { return nil }, true)
	for _, path := range []string{"/health/ready", "/v1/config"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s returned %d", path, rec.Code)
		}
		if path == "/v1/config" && !strings.Contains(rec.Body.String(), `"salesEnabled":true`) {
			t.Fatal("configured flag was not returned")
		}
	}
}
