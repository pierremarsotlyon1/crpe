// Package storeconfig wires native billing identically in the API and durable worker.
package storeconfig

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pierremarsotlyon1/crpe/backend/internal/storeapple"
	"github.com/pierremarsotlyon1/crpe/backend/internal/storebilling"
	"github.com/pierremarsotlyon1/crpe/backend/internal/storegoogle"
	"github.com/pierremarsotlyon1/crpe/backend/internal/storeprovider"
)

func New(ctx context.Context, db *pgxpool.Pool, get func(string) string) (*storebilling.Service, error) {
	providers := map[string]storeprovider.Provider{}
	a, e := storeapple.FromEnv(get)
	if e != nil {
		return nil, e
	}
	if a != nil {
		providers["apple"] = a
	}
	g, e := storegoogle.FromEnv(get)
	if e != nil {
		return nil, e
	}
	if g != nil {
		providers["google"] = g
	}
	service, e := storebilling.New(db, get, providers)
	if e != nil {
		return nil, e
	}
	mode, e := environment(get)
	if e != nil {
		return nil, e
	}
	if mode != "" {
		if e = bind(ctx, db, mode); e != nil {
			return nil, e
		}
	}
	return service, nil
}

// A deployment can explicitly pin its environment even when sales and all providers are disabled.
// Live/test Stripe credentials also define the expected mode of the shared membership database.
func environment(get func(string) string) (string, error) {
	stripe := ""
	key := get("STRIPE_SECRET_KEY")
	switch {
	case strings.HasPrefix(key, "sk_live_") || strings.HasPrefix(key, "rk_live_"):
		stripe = "Production"
	case strings.HasPrefix(key, "sk_test_") || strings.HasPrefix(key, "rk_test_"):
		stripe = "Sandbox"
	}
	mode := ""
	for _, value := range []string{get("STORE_ENVIRONMENT"), get("APPLE_STORE_ENVIRONMENT"), get("GOOGLE_PLAY_ENVIRONMENT"), stripe} {
		if value == "" {
			continue
		}
		if (value != "Sandbox" && value != "Production") || (mode != "" && mode != value) {
			return "", errors.New("incompatible store and billing environments")
		}
		mode = value
	}
	return mode, nil
}
func bind(ctx context.Context, db *pgxpool.Pool, expected string) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `INSERT INTO store_deployment(singleton,environment) VALUES(true,$1) ON CONFLICT(singleton) DO NOTHING`, expected); e != nil {
			return e
		}
		var compatible bool
		e := tx.QueryRow(ctx, `SELECT environment=$1 AND NOT EXISTS(SELECT FROM store_memberships WHERE environment<>$1) FROM store_deployment WHERE singleton FOR UPDATE`, expected).Scan(&compatible)
		if e != nil {
			return e
		}
		if !compatible {
			return errors.New("store environment differs from the membership database")
		}
		return nil
	})
}
