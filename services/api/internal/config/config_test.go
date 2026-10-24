package config

import (
	"strings"
	"testing"
	"time"
)

func base() map[string]string {
	return map[string]string{"API_ORIGIN": "https://localhost:8443", "API_AUTH_ADDRESS": "127.0.0.1:8082", "API_AUTH_CA": "ca", "API_AUTH_CERT": "cert", "API_AUTH_KEY": "key"}
}
func TestFromEnv(t *testing.T) {
	c, err := FromEnv(func(k string) string { return base()[k] })
	if err != nil || c.Address != "127.0.0.1:8080" || c.RPCDeadline != 3*time.Second || c.SlowDeadline != 10*time.Second {
		t.Fatalf("defaults: %+v %v", c, err)
	}
	for _, entry := range []struct{ key, value string }{
		{"API_ORIGIN", ""}, {"API_ORIGIN", "http://localhost"}, {"API_ORIGIN", "https://user:pass@localhost"}, {"API_ORIGIN", "https://localhost/path"}, {"API_ORIGIN", "https://localhost?secret=1"}, {"API_ORIGIN", "https://localhost#secret"}, {"API_ORIGIN", "://%"},
		{"API_AUTH_ADDRESS", "host"}, {"API_AUTH_ADDRESS", "127.0.0.1:0"}, {"API_AUTH_ADDRESS", "127.0.0.1:65536"}, {"API_ADDRESS", ":8000"}, {"API_ADDRESS", "0.0.0.0:8000"}, {"API_ADDRESS", "localhost:8000"},
		{"API_AUTH_CA", ""}, {"API_AUTH_CERT", ""}, {"API_AUTH_KEY", ""}, {"API_HTTP_CERT", "cert"}, {"API_HTTP_KEY", "key"},
		{"API_RPC_TIMEOUT", "0s"}, {"API_SLOW_TIMEOUT", "bad"}, {"API_SHUTDOWN_TIMEOUT", "2m"},
	} {
		t.Run(entry.key+entry.value, func(t *testing.T) {
			env := base()
			env[entry.key] = entry.value
			_, err := FromEnv(func(k string) string { return env[k] })
			if err == nil || strings.Contains(err.Error(), "user:pass") {
				t.Fatal("invalid config accepted or leaked")
			}
		})
	}
	env := base()
	env["API_HTTP_CERT"] = "cert"
	env["API_HTTP_KEY"] = "key"
	env["API_ADDRESS"] = "0.0.0.0:8443"
	env["API_RPC_TIMEOUT"] = "100ms"
	c, err = FromEnv(func(k string) string { return env[k] })
	if err != nil || c.RPCDeadline != 100*time.Millisecond {
		t.Fatal(c, err)
	}
	c.RPCDeadline = 0
	if c.Validate() == nil {
		t.Fatal("zero deadline accepted")
	}
	for k, v := range base() {
		t.Setenv(k, v)
	}
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
}
