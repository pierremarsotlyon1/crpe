package config

import "testing"

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr bool
	}{
		{name: "requires database", env: map[string]string{}, wantErr: true},
		{name: "safe defaults", env: map[string]string{"DATABASE_URL": "postgres://local/db"}},
		{name: "invalid flag", env: map[string]string{"DATABASE_URL": "db", "SALES_ENABLED": "yes"}, wantErr: true},
		{name: "negative pool", env: map[string]string{"DATABASE_URL": "db", "DATABASE_MAX_CONNS": "-1"}, wantErr: true},
		{name: "excessive pool", env: map[string]string{"DATABASE_URL": "db", "DATABASE_MAX_CONNS": "101"}, wantErr: true},
		{name: "overflow", env: map[string]string{"DATABASE_URL": "db", "DATABASE_MAX_CONNS": "9999999999999"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(func(key string) string { return tt.env[key] })
			if (err != nil) != tt.wantErr {
				t.Fatalf("Load error = %v, want error = %v", err, tt.wantErr)
			}
			if !tt.wantErr && (cfg.SalesEnabled || cfg.DatabaseMaxConns != 5 || cfg.HTTPAddr != ":8080") {
				t.Fatalf("unsafe defaults: %+v", cfg)
			}
		})
	}
}
