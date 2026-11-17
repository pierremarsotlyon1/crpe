package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pierremarsotlyon1/crpe/backend/internal/billing"
	"github.com/pierremarsotlyon1/crpe/backend/internal/config"
	"github.com/pierremarsotlyon1/crpe/backend/internal/database"
	"github.com/pierremarsotlyon1/crpe/backend/internal/dialog"
	"github.com/pierremarsotlyon1/crpe/backend/internal/httpapi"
	"github.com/pierremarsotlyon1/crpe/backend/internal/learning"
	"github.com/pierremarsotlyon1/crpe/backend/internal/mailer"
	"github.com/pierremarsotlyon1/crpe/backend/internal/objectstore"
	"github.com/pierremarsotlyon1/crpe/backend/internal/storeconfig"
	"github.com/pierremarsotlyon1/crpe/backend/internal/voice"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("api stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := database.Open(ctx, cfg.DatabaseURL, cfg.DatabaseMaxConns)
	if err != nil {
		return err
	}
	defer pool.Close()
	stores, err := storeconfig.New(ctx, pool, os.Getenv)
	if err != nil {
		return err
	}
	store, err := objectstore.FromEnv(os.Getenv)
	if err != nil {
		return err
	}
	mail, err := mailer.FromEnv(os.Getenv)
	if err != nil {
		return err
	}
	webURL := os.Getenv("PUBLIC_WEB_URL")
	if webURL == "" {
		webURL = "http://localhost:3000"
	}
	mobileURL := os.Getenv("MOBILE_AUTH_URL")
	if mobileURL == "" {
		mobileURL = "crpemaths://auth"
	}
	var admins []string
	if list := os.Getenv("ADMIN_EMAILS"); list != "" {
		admins = strings.Split(list, ",")
	}
	speaker, err := voice.FromEnv(os.Getenv)
	if err != nil {
		return err
	}
	model, err := dialog.FromEnv(os.Getenv)
	if err != nil {
		return err
	}
	commerce, err := billing.New(pool, os.Getenv)
	if err != nil {
		return err
	}
	var dialogue learning.StructuredDialog
	if model != nil {
		dialogue = model
	}
	var trusted []netip.Prefix
	if raw := os.Getenv("TRUSTED_PROXY_CIDRS"); raw != "" {
		for _, item := range strings.Split(raw, ",") {
			cidr, err := netip.ParsePrefix(strings.TrimSpace(item))
			if err != nil {
				return fmt.Errorf("invalid TRUSTED_PROXY_CIDRS")
			}
			trusted = append(trusted, cidr)
		}
	}
	var ipLimit int
	if raw := os.Getenv("AUTH_IP_HOURLY_LIMIT"); raw != "" {
		ipLimit, err = strconv.Atoi(raw)
		if err != nil || ipLimit < 1 {
			return fmt.Errorf("invalid AUTH_IP_HOURLY_LIMIT")
		}
	}
	app, err := learning.New(pool, mail, store, learning.Options{Stores: stores, TrustedProxyCIDRs: trusted, AuthIPHourlyLimit: ipLimit, Dialog: dialogue, Billing: commerce, Voice: speaker, WebURL: webURL, MobileURL: mobileURL, AdminEmails: admins, SalesEnabled: cfg.SalesEnabled, CorrectionEnabled: model != nil || os.Getenv("CORRECTION_MODE") == "local"}, logger)
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.New(func(ctx context.Context) error { return database.Ready(ctx, pool) }, cfg.SalesEnabled, app.Register),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      75 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.ListenAndServe() }()
	logger.Info("api started", "address", cfg.HTTPAddr, "sales_enabled", cfg.SalesEnabled)
	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return err
		}
	}
	logger.Info("api drained")
	return nil
}
