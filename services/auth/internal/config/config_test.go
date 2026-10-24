package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

func TestConfigRejectsUnsafeSettingsWithoutEchoingSecrets(t *testing.T) {
	for _, tc := range []struct{ name, key, value string }{
		{"missing URL", "AUTH_DATABASE_URL", ""},
		{"malformed URL", "AUTH_DATABASE_URL", "://TOP_SECRET"},
		{"implicit TLS", "AUTH_DATABASE_URL", "postgres://user:TOP_SECRET@localhost/auth"},
		{"prefer TLS", "AUTH_DATABASE_URL", "postgres://user:TOP_SECRET@localhost/auth?sslmode=prefer"},
		{"production plaintext", "AUTH_ENV", "production"},
		{"remote plaintext", "AUTH_DATABASE_URL", "postgres://user:TOP_SECRET@example.com/auth?sslmode=disable"},
		{"host override", "AUTH_DATABASE_URL", "postgres://user:TOP_SECRET@localhost/auth?sslmode=disable&host=example.com"},
		{"unknown environment", "AUTH_ENV", "TOP_SECRET"},
		{"bad HTTP", "AUTH_HTTP_ADDRESS", "TOP_SECRET"},
		{"bad pool", "AUTH_DB_MAX_CONNECTIONS", "0"},
		{"large pool", "AUTH_DB_MAX_CONNECTIONS", "101"},
		{"negative timeout", "AUTH_QUERY_TIMEOUT", "-1s"},
		{"large timeout", "AUTH_STARTUP_TIMEOUT", "10m"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := map[string]string{"AUTH_ENV": "local", "AUTH_DATABASE_URL": "postgres://user:TOP_SECRET@127.0.0.1/auth?sslmode=disable"}
			env[tc.key] = tc.value
			_, err := Load(func(k string) string { return env[k] })
			if err == nil {
				t.Fatal("unsafe config accepted")
			}
			if strings.Contains(err.Error(), "TOP_SECRET") {
				t.Fatal("config error exposed secret")
			}
		})
	}
}

func TestConfigDefaultsTLSAndRedaction(t *testing.T) {
	cfg, err := Load(func(k string) string {
		if k == "AUTH_DATABASE_URL" {
			return "postgres://user:TOP_SECRET@db.example/auth?sslmode=verify-full"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := cfg.PoolConfig()
	if err != nil {
		t.Fatal(err)
	}
	if pool.ConnConfig.TLSConfig == nil || pool.ConnConfig.TLSConfig.InsecureSkipVerify {
		t.Fatal("TLS verification disabled")
	}
	if pool.MaxConns != 5 || cfg.HTTPAddress != "127.0.0.1:8082" {
		t.Fatal("unexpected defaults")
	}
	var log bytes.Buffer
	slog.New(slog.NewJSONHandler(&log, nil)).Info("config", "settings", cfg)
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, rendered := range []string{fmt.Sprintf("%v %+v %#v", cfg, cfg, cfg), log.String(), string(data)} {
		if strings.Contains(rendered, "TOP_SECRET") || strings.Contains(rendered, "postgres://") {
			t.Fatal("configuration exposed database URL")
		}
	}
}
