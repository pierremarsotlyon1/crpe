// Package config validates process configuration before opening dependencies.
package config

import (
	"fmt"
	"strconv"
)

type Config struct {
	DatabaseURL      string
	HTTPAddr         string
	DatabaseMaxConns int32
	SalesEnabled     bool
}

func Load(lookup func(string) string) (Config, error) {
	cfg := Config{
		DatabaseURL:      lookup("DATABASE_URL"),
		HTTPAddr:         ":8080",
		DatabaseMaxConns: 5,
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if value := lookup("HTTP_ADDR"); value != "" {
		cfg.HTTPAddr = value
	}
	if value := lookup("DATABASE_MAX_CONNS"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 32)
		if err != nil || parsed < 1 || parsed > 100 {
			return Config{}, fmt.Errorf("DATABASE_MAX_CONNS must be between 1 and 100")
		}
		cfg.DatabaseMaxConns = int32(parsed)
	}
	if value := lookup("SALES_ENABLED"); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return Config{}, fmt.Errorf("SALES_ENABLED must be a boolean")
		}
		cfg.SalesEnabled = parsed
	}
	return cfg, nil
}
