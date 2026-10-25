package config_test

import (
	"log/slog"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/config"
)

// Each test controls all settings so shell configuration cannot change its result.
func setup(t *testing.T) {
	t.Helper()
	for name, value := range map[string]string{
		"CORE_LOG_LEVEL": "info", "CORE_LOG_FORMAT": "json",
		"CORE_POSTGRES_HOST": "unreachable.invalid", "CORE_POSTGRES_PORT": "5432",
		"CORE_POSTGRES_USER": "core", "CORE_POSTGRES_PASSWORD": "private-secret",
		"CORE_POSTGRES_DATABASE": "core", "CORE_POSTGRES_SSL_MODE": "verify-full",
		"CORE_POSTGRES_MAX_CONNS": "10", "CORE_POSTGRES_MAX_IDLE_CONNS": "0",
		"CORE_POSTGRES_CONNECT_TIMEOUT": "5s", "CORE_POSTGRES_QUERY_TIMEOUT": "5s",
		"CORE_SHUTDOWN_TIMEOUT": "10s",
	} {
		t.Setenv(name, value)
	}
}

func TestLoadDefaults(t *testing.T) {
	setup(t)
	for _, name := range []string{
		"CORE_LOG_LEVEL", "CORE_LOG_FORMAT", "CORE_POSTGRES_MAX_CONNS", "CORE_POSTGRES_MAX_IDLE_CONNS",
		"CORE_POSTGRES_CONNECT_TIMEOUT", "CORE_POSTGRES_QUERY_TIMEOUT", "CORE_SHUTDOWN_TIMEOUT",
		"CORE_POSTGRES_PORT",
	} {
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Log.Level != slog.LevelInfo || cfg.Log.Format != "json" ||
		cfg.Postgres.Port != 5432 || cfg.Postgres.MaxConns != 10 || cfg.Postgres.MaxIdleConns != 2 ||
		cfg.Postgres.ConnectTimeout != 5*time.Second || cfg.Postgres.QueryTimeout != 5*time.Second ||
		cfg.ShutdownTimeout != 10*time.Second {
		t.Fatal("defaults were not applied")
	}
}

func TestLoadOverrides(t *testing.T) {
	setup(t)
	t.Setenv("CORE_LOG_LEVEL", "DEBUG")
	t.Setenv("CORE_LOG_FORMAT", " TEXT ")
	t.Setenv("CORE_POSTGRES_MAX_CONNS", "7")
	t.Setenv("CORE_POSTGRES_MAX_IDLE_CONNS", "3")
	t.Setenv("CORE_POSTGRES_CONNECT_TIMEOUT", "800ms")
	t.Setenv("CORE_POSTGRES_QUERY_TIMEOUT", "2s")
	t.Setenv("CORE_SHUTDOWN_TIMEOUT", "3s")
	t.Setenv("CORE_POSTGRES_HOST", "::1")
	t.Setenv("CORE_POSTGRES_PORT", "5433")
	t.Setenv("CORE_POSTGRES_USER", "custom-user")
	t.Setenv("CORE_POSTGRES_PASSWORD", " p@ss:/?#% + ")
	t.Setenv("CORE_POSTGRES_DATABASE", "custom-db")
	t.Setenv("CORE_POSTGRES_SSL_MODE", "require")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Log.Level != slog.LevelDebug || cfg.Log.Format != "text" ||
		cfg.Postgres.Host != "::1" || cfg.Postgres.Port != 5433 || cfg.Postgres.User != "custom-user" ||
		cfg.Postgres.Password != " p@ss:/?#% + " || cfg.Postgres.Database != "custom-db" ||
		cfg.Postgres.SSLMode != "require" ||
		cfg.Postgres.MaxConns != 7 || cfg.Postgres.MaxIdleConns != 3 ||
		cfg.Postgres.ConnectTimeout != 800*time.Millisecond || cfg.Postgres.QueryTimeout != 2*time.Second ||
		cfg.ShutdownTimeout != 3*time.Second {
		t.Fatal("environment overrides were not applied")
	}
}

func TestLoadZeroIdleLimit(t *testing.T) {
	setup(t)
	t.Setenv("CORE_POSTGRES_MAX_IDLE_CONNS", "0")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Postgres.MaxIdleConns != 0 {
		t.Fatal("explicit zero idle limit replaced with default")
	}
}

func TestLoadInvalid(t *testing.T) {
	for _, tc := range []struct {
		name, setting, value string
	}{
		{"unknown level", "CORE_LOG_LEVEL", "private-secret"},
		{"empty level", "CORE_LOG_LEVEL", ""},
		{"unsupported format", "CORE_LOG_FORMAT", "private-secret"},
		{"empty format", "CORE_LOG_FORMAT", ""},
		{"missing host", "CORE_POSTGRES_HOST", ""},
		{"invalid host", "CORE_POSTGRES_HOST", "private-secret/invalid"},
		{"bracketed IPv6", "CORE_POSTGRES_HOST", "[::1]"},
		{"host includes port", "CORE_POSTGRES_HOST", "host:5432"},
		{"missing user", "CORE_POSTGRES_USER", ""},
		{"missing password", "CORE_POSTGRES_PASSWORD", ""},
		{"missing database", "CORE_POSTGRES_DATABASE", ""},
		{"missing sslmode", "CORE_POSTGRES_SSL_MODE", ""},
		{"invalid sslmode", "CORE_POSTGRES_SSL_MODE", "private-secret"},
		{"empty port", "CORE_POSTGRES_PORT", ""},
		{"invalid port", "CORE_POSTGRES_PORT", "private-secret"},
		{"zero port", "CORE_POSTGRES_PORT", "0"},
		{"negative port", "CORE_POSTGRES_PORT", "-1"},
		{"port overflow", "CORE_POSTGRES_PORT", "65536"},
		{"zero pool", "CORE_POSTGRES_MAX_CONNS", "0"},
		{"negative pool", "CORE_POSTGRES_MAX_IDLE_CONNS", "-1"},
		{"inverted pool", "CORE_POSTGRES_MAX_IDLE_CONNS", "11"},
		{"pool overflow", "CORE_POSTGRES_MAX_CONNS", "2147483648"},
		{"invalid pool", "CORE_POSTGRES_MAX_CONNS", "private-secret"},
		{"zero timeout", "CORE_POSTGRES_CONNECT_TIMEOUT", "0s"},
		{"negative timeout", "CORE_POSTGRES_QUERY_TIMEOUT", "-1s"},
		{"invalid timeout", "CORE_SHUTDOWN_TIMEOUT", "private-secret"},
		{"overflow timeout", "CORE_SHUTDOWN_TIMEOUT", "999999999999h"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setup(t)
			t.Setenv(tc.setting, tc.value)
			cfg, err := config.Load()
			if err == nil {
				t.Fatal("invalid setting accepted")
			}
			if !strings.Contains(err.Error(), tc.setting) || strings.Contains(err.Error(), "private-secret") {
				t.Fatalf("unsafe or unhelpful error: %s", err)
			}
			if !reflect.DeepEqual(cfg, config.Config{}) {
				t.Fatal("failed load returned partially usable config")
			}
		})
	}
}

func TestPostgresDSN(t *testing.T) {
	t.Parallel()
	for _, host := range []string{"localhost", "127.0.0.1", "::1"} {
		t.Run(host, func(t *testing.T) {
			t.Parallel()
			pg := config.Postgres{
				Host: host, Port: 5433,
				User: "u@name", Password: " p@ss:/?#% + ",
				Database: "core/db name", SSLMode: "verify-full",
			}
			u, err := url.Parse(pg.DSN())
			if err != nil {
				t.Fatal("generated DSN is not a valid URL")
			}
			password, ok := u.User.Password()
			if !ok || password != pg.Password || u.User.Username() != pg.User {
				t.Fatal("credentials did not survive URL encoding")
			}
			if u.Hostname() != host || u.Port() != "5433" || u.Path != "/"+pg.Database {
				t.Fatal("host, port or database changed during URL encoding")
			}
			if u.Query().Get("sslmode") != pg.SSLMode || u.Fragment != "" {
				t.Fatal("SSL mode was ignored or credentials escaped the URL fields")
			}
		})
	}
}
