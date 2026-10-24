package config

import (
	"errors"
	"net"
	"net/url"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Address, Origin, AuthAddress, CA, Cert, Key, HTTPCert, HTTPKey string
	CoreAddress, CoreServerName, CoreCA, CoreCert, CoreKey         string
	ImportSpoolDir                                                 string
	RPCDeadline, SlowDeadline, ShutdownTimeout                     time.Duration
}

func Load() (Config, error) { return FromEnv(os.Getenv) }

func FromEnv(get func(string) string) (Config, error) {
	c := Config{
		ImportSpoolDir: get("API_IMPORT_SPOOL_DIR"),
		CoreAddress:    get("API_CORE_ADDRESS"), CoreServerName: get("API_CORE_SERVER_NAME"),
		CoreCA: get("API_CORE_CA"), CoreCert: get("API_CORE_CERT"), CoreKey: get("API_CORE_KEY"),
		Address: get("API_ADDRESS"), Origin: get("API_ORIGIN"), AuthAddress: get("API_AUTH_ADDRESS"),
		CA: get("API_AUTH_CA"), Cert: get("API_AUTH_CERT"), Key: get("API_AUTH_KEY"),
		HTTPCert: get("API_HTTP_CERT"), HTTPKey: get("API_HTTP_KEY"),
		RPCDeadline: 3 * time.Second, SlowDeadline: 10 * time.Second, ShutdownTimeout: 5 * time.Second,
	}
	if c.Address == "" {
		c.Address = "127.0.0.1:8080"
	}
	for name, target := range map[string]*time.Duration{"API_RPC_TIMEOUT": &c.RPCDeadline, "API_SLOW_TIMEOUT": &c.SlowDeadline, "API_SHUTDOWN_TIMEOUT": &c.ShutdownTimeout} {
		if raw := get(name); raw != "" {
			d, err := time.ParseDuration(raw)
			if err != nil || d < time.Millisecond || d > time.Minute {
				return Config{}, errors.New("invalid timeout configuration")
			}
			*target = d
		}
	}
	if c.CoreAddress != "" {
		if c.CoreCA == "" {
			c.CoreCA = c.CA
		}
		if c.CoreCert == "" {
			c.CoreCert = c.Cert
		}
		if c.CoreKey == "" {
			c.CoreKey = c.Key
		}
	}
	return c, c.Validate()
}

func (c Config) Validate() error {
	u, err := url.Parse(c.Origin)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return errors.New("API_ORIGIN must be an exact HTTPS origin")
	}
	if c.CoreAddress != "" {
		host, port, err := net.SplitHostPort(c.CoreAddress)
		n, numberErr := strconv.Atoi(port)
		if err != nil || numberErr != nil || host == "" || n < 1 || n > 65535 || c.CoreCA == "" || c.CoreCert == "" || c.CoreKey == "" {
			return errors.New("invalid Core address or mTLS configuration")
		}
	} else if c.CoreServerName != "" || c.CoreCA != "" || c.CoreCert != "" || c.CoreKey != "" {
		return errors.New("core TLS configuration requires API_CORE_ADDRESS")
	}
	for _, address := range []string{c.Address, c.AuthAddress} {
		host, port, splitErr := net.SplitHostPort(address)
		n, numberErr := strconv.Atoi(port)
		if splitErr != nil || numberErr != nil || host == "" || n < 0 || n > 65535 || (address == c.AuthAddress && n == 0) {
			return errors.New("invalid listener or Auth address")
		}
	}
	if c.CA == "" || c.Cert == "" || c.Key == "" {
		return errors.New("auth mTLS files are required")
	}
	if (c.HTTPCert == "") != (c.HTTPKey == "") {
		return errors.New("HTTP TLS identity requires both certificate and key")
	}
	if c.HTTPCert == "" {
		host, _, _ := net.SplitHostPort(c.Address)
		if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
			return errors.New("plaintext HTTP is restricted to loopback behind a trusted TLS proxy")
		}
	}
	if c.RPCDeadline <= 0 || c.SlowDeadline <= 0 || c.ShutdownTimeout <= 0 {
		return errors.New("timeouts must be positive")
	}
	return nil
}
