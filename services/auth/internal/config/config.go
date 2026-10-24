// Package config loads validated environment settings without logging secrets.
package config

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	databaseURL     string
	Environment     string
	HTTPAddress     string
	StartupTimeout  time.Duration
	QueryTimeout    time.Duration
	ShutdownTimeout time.Duration
	MaxConnections  int32
}

func (c Config) Format(w fmt.State, _ rune) { _, _ = io.WriteString(w, "AuthConfig{secrets:redacted}") }

func (c Config) LogValue() slog.Value {
	return slog.GroupValue(slog.String("environment", c.Environment), slog.String("http_address", c.HTTPAddress))
}

func Load(getenv func(string) string) (Config, error) {
	c := Config{databaseURL: getenv("AUTH_DATABASE_URL"), Environment: "production", HTTPAddress: "127.0.0.1:8082",
		StartupTimeout: 5 * time.Second, QueryTimeout: time.Second, ShutdownTimeout: 5 * time.Second, MaxConnections: 5}
	if value := getenv("AUTH_ENV"); value != "" {
		c.Environment = value
	}
	if c.Environment != "production" && c.Environment != "local" {
		return Config{}, errors.New("AUTH_ENV must be production or local")
	}
	if value := getenv("AUTH_HTTP_ADDRESS"); value != "" {
		c.HTTPAddress = value
	}
	host, portText, err := net.SplitHostPort(c.HTTPAddress)
	if err != nil || (host != "localhost" && net.ParseIP(host) == nil) {
		return Config{}, errors.New("AUTH_HTTP_ADDRESS requires a literal IP or localhost and port")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 0 || port > 65535 || (port == 0 && c.Environment != "local") {
		return Config{}, errors.New("AUTH_HTTP_ADDRESS has an invalid port")
	}
	for _, setting := range []struct {
		name   string
		target *time.Duration
	}{
		{"AUTH_STARTUP_TIMEOUT", &c.StartupTimeout}, {"AUTH_QUERY_TIMEOUT", &c.QueryTimeout}, {"AUTH_SHUTDOWN_TIMEOUT", &c.ShutdownTimeout},
	} {
		if value := getenv(setting.name); value != "" {
			duration, err := time.ParseDuration(value)
			if err != nil || duration < 50*time.Millisecond || duration > 5*time.Minute {
				return Config{}, fmt.Errorf("%s must be between 50ms and 5m", setting.name)
			}
			*setting.target = duration
		}
	}
	if value := getenv("AUTH_DB_MAX_CONNECTIONS"); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 || n > 100 {
			return Config{}, errors.New("AUTH_DB_MAX_CONNECTIONS must be between 1 and 100")
		}
		c.MaxConnections = int32(n)
	}
	if _, err := c.PoolConfig(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// PoolConfig rejects implicit TLS fallback. Plaintext is an explicit local-only
// choice, limited to a loopback server; production always verifies its identity.
func (c Config) PoolConfig() (*pgxpool.Config, error) {
	u, err := url.Parse(c.databaseURL)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" || u.Path == "" || u.Path == "/" || u.User == nil || u.User.Username() == "" || u.Fragment != "" {
		return nil, errors.New("AUTH_DATABASE_URL must be a PostgreSQL URL with host, database and user")
	}
	mode := u.Query().Get("sslmode")
	for key := range u.Query() {
		switch key {
		case "sslmode", "sslrootcert", "sslcert", "sslkey", "connect_timeout", "search_path", "application_name":
		default:
			return nil, errors.New("AUTH_DATABASE_URL contains an unsupported parameter")
		}
	}
	local := u.Hostname() == "localhost" || (net.ParseIP(u.Hostname()) != nil && net.ParseIP(u.Hostname()).IsLoopback())
	if mode != "verify-full" && !(mode == "disable" && c.Environment == "local" && local) {
		return nil, errors.New("AUTH_DATABASE_URL requires sslmode=verify-full (disable is local loopback only)")
	}
	pool, err := pgxpool.ParseConfig(c.databaseURL)
	if err != nil {
		return nil, errors.New("AUTH_DATABASE_URL or TLS configuration is invalid")
	}
	if mode == "disable" {
		for _, endpoint := range append([]string{pool.ConnConfig.Host}, fallbackHosts(pool)...) {
			ip := net.ParseIP(endpoint)
			if endpoint != "localhost" && (ip == nil || !ip.IsLoopback()) {
				return nil, errors.New("plaintext database must use loopback")
			}
		}
	}
	if mode == "verify-full" {
		if pool.ConnConfig.TLSConfig == nil || pool.ConnConfig.TLSConfig.InsecureSkipVerify {
			return nil, errors.New("database TLS must verify the server")
		}
		for _, fallback := range pool.ConnConfig.Fallbacks {
			if fallback.TLSConfig == nil || fallback.TLSConfig.InsecureSkipVerify {
				return nil, errors.New("database TLS fallback is forbidden")
			}
		}
	}
	pool.MaxConns = c.MaxConnections
	pool.MinConns = 0
	pool.MaxConnLifetime = 30 * time.Minute
	pool.MaxConnIdleTime = 5 * time.Minute
	pool.ConnConfig.ConnectTimeout = c.StartupTimeout
	pool.ConnConfig.RuntimeParams["application_name"] = "saldo-auth"
	return pool, nil
}

func fallbackHosts(pool *pgxpool.Config) []string {
	hosts := make([]string, 0, len(pool.ConnConfig.Fallbacks))
	for _, fallback := range pool.ConnConfig.Fallbacks {
		hosts = append(hosts, fallback.Host)
	}
	return hosts
}
