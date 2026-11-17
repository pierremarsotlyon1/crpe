// Package httpapi exposes the public HTTP contract without instance-local state.
package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type Readiness func(context.Context) error

func New(ready Readiness, salesEnabled bool, registrars ...func(*http.ServeMux)) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := ready(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /v1/config", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, struct {
			SalesEnabled bool `json:"salesEnabled"`
		}{SalesEnabled: salesEnabled})
	})
	for _, register := range registrars {
		register(mux)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Socket write deadlines do not cancel database/provider calls. Bound the
		// request context too, leaving time before the server's 75s write deadline.
		ctx, cancel := context.WithTimeout(r.Context(), 65*time.Second)
		defer cancel()
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	// These fixed response types always encode. A write failure means the client disconnected.
	_ = json.NewEncoder(w).Encode(body)
}
