// Package config loads and validates environment configuration without opening resources.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	errInvalidPostgresHost = errors.New("CORE_POSTGRES_HOST must be a hostname or an unbracketed IP address")
	errInvalidPostgresPort = errors.New("CORE_POSTGRES_PORT must be an integer from 1 to 65535")
	errInvalidSSLMode      = errors.New(
		"CORE_POSTGRES_SSL_MODE must be disable, allow, prefer, require, verify-ca or verify-full",
	)
)

// Config contains the settings needed by the Core foundation.
type Config struct {
	Log             Log
	Postgres        Postgres
	ShutdownTimeout time.Duration
	FixtureCoverage []FixtureCoverage
}

// Log configures a caller-owned slog logger.
type Log struct {
	Level  slog.Level
	Format string
}

// Postgres contains connection settings; loading them does not contact the database.
// Password and the generated DSN must not be logged, including as part of Config.
type Postgres struct {
	Host           string
	Port           uint16
	User           string
	Password       string
	Database       string
	SSLMode        string
	MaxConns       int32
	MaxIdleConns   int32
	ConnectTimeout time.Duration
	QueryTimeout   time.Duration
}

// Load applies defaults, environment overrides and validation in that order.
// Errors identify the setting without echoing its potentially sensitive value.
func Load() (Config, error) {
	cfg := Config{
		Log: Log{Level: slog.LevelInfo, Format: "json"},
		Postgres: Postgres{
			Host: os.Getenv("CORE_POSTGRES_HOST"), Port: 5432,
			User: os.Getenv("CORE_POSTGRES_USER"), Password: os.Getenv("CORE_POSTGRES_PASSWORD"),
			Database: os.Getenv("CORE_POSTGRES_DATABASE"), SSLMode: os.Getenv("CORE_POSTGRES_SSL_MODE"),
			MaxConns:       10,
			MaxIdleConns:   2,
			ConnectTimeout: 5 * time.Second, QueryTimeout: 5 * time.Second,
		},
		ShutdownTimeout: 10 * time.Second,
	}
	if value, ok := os.LookupEnv("CORE_LOG_LEVEL"); ok {
		if err := cfg.Log.Level.UnmarshalText([]byte(strings.TrimSpace(value))); err != nil {
			return Config{}, errors.New("CORE_LOG_LEVEL must be a valid slog level")
		}
	}
	if value, ok := os.LookupEnv("CORE_LOG_FORMAT"); ok {
		cfg.Log.Format = strings.ToLower(strings.TrimSpace(value))
	}
	if cfg.Log.Format != "json" && cfg.Log.Format != "text" {
		return Config{}, errors.New("CORE_LOG_FORMAT must be json or text")
	}
	if value, ok := os.LookupEnv("CORE_POSTGRES_PORT"); ok {
		port, err := strconv.ParseUint(value, 10, 16)
		if err != nil || port == 0 {
			return Config{}, errInvalidPostgresPort
		}
		cfg.Postgres.Port = uint16(port)
	}
	if err := cfg.Postgres.validate(); err != nil {
		return Config{}, err
	}
	for _, setting := range []struct {
		name   string
		target *int32
	}{
		{"CORE_POSTGRES_MAX_CONNS", &cfg.Postgres.MaxConns},
		{"CORE_POSTGRES_MAX_IDLE_CONNS", &cfg.Postgres.MaxIdleConns},
	} {
		if value, ok := os.LookupEnv(setting.name); ok {
			n, err := strconv.ParseInt(value, 10, 32)
			if err != nil || n < 0 {
				return Config{}, fmt.Errorf("%s must be a nonnegative int32", setting.name)
			}
			*setting.target = int32(n)
		}
	}
	if cfg.Postgres.MaxConns == 0 || cfg.Postgres.MaxIdleConns > cfg.Postgres.MaxConns {
		return Config{}, errors.New("CORE_POSTGRES_MAX_CONNS must be positive and at least CORE_POSTGRES_MAX_IDLE_CONNS")
	}
	for _, setting := range []struct {
		name   string
		target *time.Duration
	}{
		{"CORE_POSTGRES_CONNECT_TIMEOUT", &cfg.Postgres.ConnectTimeout},
		{"CORE_POSTGRES_QUERY_TIMEOUT", &cfg.Postgres.QueryTimeout},
		{"CORE_SHUTDOWN_TIMEOUT", &cfg.ShutdownTimeout},
	} {
		if value, ok := os.LookupEnv(setting.name); ok {
			duration, err := time.ParseDuration(value)
			if err != nil || duration <= 0 {
				return Config{}, fmt.Errorf("%s must be a positive duration", setting.name)
			}
			*setting.target = duration
		}
	}
	enabledRaw := os.Getenv("CORE_DATASET_FIXTURE_COVERAGE_ENABLED")
	if enabledRaw != "" && enabledRaw != "true" && enabledRaw != "false" {
		return Config{}, errors.New("invalid fixture coverage enablement")
	}
	entries, err := LoadFixtureCoverage(os.Getenv("CORE_DATASET_FIXTURE_MANIFEST"), enabledRaw == "true")
	if err != nil {
		return Config{}, err
	}
	cfg.FixtureCoverage = entries
	return cfg, nil
}

// DSN builds a connection URL from validated settings, escaping credentials and database.
// The returned string contains the password and must not be logged.
func (pg Postgres) DSN() string {
	u := url.URL{
		Scheme:  "postgres",
		User:    url.UserPassword(pg.User, pg.Password),
		Host:    net.JoinHostPort(pg.Host, strconv.FormatUint(uint64(pg.Port), 10)),
		Path:    "/" + pg.Database,
		RawPath: "/" + url.PathEscape(pg.Database),
	}
	query := url.Values{}
	query.Set("sslmode", pg.SSLMode)
	u.RawQuery = query.Encode()
	return u.String()
}

func (pg Postgres) validate() error {
	for _, field := range []struct {
		name  string
		value string
	}{
		{"CORE_POSTGRES_HOST", pg.Host},
		{"CORE_POSTGRES_USER", pg.User},
		{"CORE_POSTGRES_PASSWORD", pg.Password},
		{"CORE_POSTGRES_DATABASE", pg.Database},
	} {
		if field.value == "" {
			return fmt.Errorf("%s is required", field.name)
		}
	}
	invalidHostChars := strings.ContainsAny(pg.Host, " \t\r\n/?#@[]\\")
	invalidIP := strings.Contains(pg.Host, ":") && net.ParseIP(pg.Host) == nil
	if invalidHostChars || invalidIP {
		return errInvalidPostgresHost
	}
	if pg.Port == 0 {
		return errInvalidPostgresPort
	}
	switch pg.SSLMode {
	case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
		return nil
	default:
		return errInvalidSSLMode
	}
}
