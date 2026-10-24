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

	"github.com/jackc/pgx/v5"
)

type Config struct {
	codeHMACKey           []byte
	csrfHMACKey           []byte
	GRPCAddress           string
	GRPCCertFile          string
	GRPCKeyFile           string
	GRPCCAFile            string
	DemoEmailVerification bool
	DemoTestEmails        []string
	databaseURL           string
	Environment           string
	HTTPAddress           string
	StartupTimeout        time.Duration
	QueryTimeout          time.Duration
	ShutdownTimeout       time.Duration
	MaxConnections        int
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
		c.MaxConnections = n
	}
	if err := c.loadRPC(getenv); err != nil {
		return Config{}, err
	}
	if _, err := c.ConnectionConfig(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// ConnectionConfig rejects implicit TLS fallback. Plaintext is an explicit local-only
// choice, limited to a loopback server; production always verifies its identity.
func (c Config) ConnectionConfig() (*pgx.ConnConfig, error) {
	if err := validateDatabaseURL(c.databaseURL, c.Environment); err != nil {
		return nil, err
	}
	connection, err := pgx.ParseConfig(c.databaseURL)
	if err != nil {
		return nil, errors.New("AUTH_DATABASE_URL or TLS configuration is invalid")
	}
	if err := validateConnection(connection, c.Environment); err != nil {
		return nil, err
	}
	connection.ConnectTimeout = c.StartupTimeout
	connection.RuntimeParams["application_name"] = "saldo-auth"
	return connection, nil
}

func validateDatabaseURL(raw, environment string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" || u.Path == "" || u.Path == "/" || u.User == nil || u.User.Username() == "" || u.Fragment != "" {
		return errors.New("AUTH_DATABASE_URL must be a PostgreSQL URL with host, database and user")
	}
	mode := u.Query().Get("sslmode")
	for key := range u.Query() {
		switch key {
		case "sslmode", "sslrootcert", "sslcert", "sslkey", "connect_timeout", "search_path", "application_name":
		default:
			return errors.New("AUTH_DATABASE_URL contains an unsupported parameter")
		}
	}
	local := u.Hostname() == "localhost" || (net.ParseIP(u.Hostname()) != nil && net.ParseIP(u.Hostname()).IsLoopback())
	if mode != "verify-full" && (mode != "disable" || environment != "local" || !local) {
		return errors.New("AUTH_DATABASE_URL requires sslmode=verify-full (disable is local loopback only)")
	}
	return nil
}

func validateConnection(connection *pgx.ConnConfig, environment string) error {
	if connection.TLSConfig == nil {
		if environment != "local" {
			return errors.New("database TLS must verify the server")
		}
		for _, endpoint := range append([]string{connection.Host}, fallbackHosts(connection)...) {
			ip := net.ParseIP(endpoint)
			if endpoint != "localhost" && (ip == nil || !ip.IsLoopback()) {
				return errors.New("plaintext database must use loopback")
			}
		}
		return nil
	}
	if connection.TLSConfig.InsecureSkipVerify {
		return errors.New("database TLS must verify the server")
	}
	for _, fallback := range connection.Fallbacks {
		if fallback.TLSConfig == nil || fallback.TLSConfig.InsecureSkipVerify {
			return errors.New("database TLS fallback is forbidden")
		}
	}
	return nil
}

func fallbackHosts(connection *pgx.ConnConfig) []string {
	hosts := make([]string, 0, len(connection.Fallbacks))
	for _, fallback := range connection.Fallbacks {
		hosts = append(hosts, fallback.Host)
	}
	return hosts
}
