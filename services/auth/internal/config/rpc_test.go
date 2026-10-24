package config

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

func rpcEnvironment() map[string]string {
	return map[string]string{
		"AUTH_ENV":            "local",
		"AUTH_DATABASE_URL":   "postgres://user:TOP_SECRET@127.0.0.1/auth?sslmode=disable",
		"AUTH_GRPC_ADDRESS":   "127.0.0.1:9092",
		"AUTH_GRPC_CERT_FILE": "server-cert.pem",
		"AUTH_GRPC_KEY_FILE":  "server-key.pem",
		"AUTH_GRPC_CA_FILE":   "ca.pem",
		"AUTH_CODE_HMAC_KEY":  base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32)),
		"AUTH_CSRF_HMAC_KEY":  base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32)),
	}
}

func TestRPCConfigDefaultsToStrictAndCopiesKeys(t *testing.T) {
	env := rpcEnvironment()
	cfg, err := Load(func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DemoEmailVerification || len(cfg.DemoTestEmails) != 0 {
		t.Fatal("default configuration enabled demo verification")
	}
	codeKey, csrfKey := cfg.CodeHMACKey(), cfg.CSRFHMACKey()
	if len(codeKey) != 32 || len(csrfKey) != 32 {
		t.Fatal("HMAC keys were not loaded")
	}
	codeKey[0], csrfKey[0] = 3, 4
	if cfg.CodeHMACKey()[0] != 1 || cfg.CSRFHMACKey()[0] != 2 {
		t.Fatal("caller mutated configuration key material")
	}
	var log bytes.Buffer
	slog.New(slog.NewJSONHandler(&log, nil)).Info("config", "settings", cfg)
	encoded, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, rendered := range []string{fmt.Sprintf("%v %+v %#v", cfg, cfg, cfg), log.String(), string(encoded)} {
		for _, secret := range []string{"TOP_SECRET", env["AUTH_CODE_HMAC_KEY"], env["AUTH_CSRF_HMAC_KEY"]} {
			if strings.Contains(rendered, secret) {
				t.Fatal("configuration rendered secret key material")
			}
		}
	}
}

func TestRPCConfigDemoNeedsIsolatedTestAccounts(t *testing.T) {
	env := rpcEnvironment()
	env["AUTH_DEMO_EMAIL_VERIFICATION"] = "true"
	env["AUTH_DEMO_TEST_EMAILS"] = " Owner@Example.test,financier@demo.test "
	cfg, err := Load(func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.DemoEmailVerification || len(cfg.DemoTestEmails) != 2 ||
		cfg.DemoTestEmails[0] != "owner@example.test" || cfg.DemoTestEmails[1] != "financier@demo.test" {
		t.Fatal("demo did not retain the exact normalized test-account allowlist")
	}
}

func TestRPCConfigRejectsUnsafeSettings(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values map[string]string
	}{
		{name: "unknown flag", values: map[string]string{"AUTH_DEMO_EMAIL_VERIFICATION": "TOP_SECRET"}},
		{name: "truthy integer", values: map[string]string{"AUTH_DEMO_EMAIL_VERIFICATION": "1"}},
		{name: "production demo", values: map[string]string{"AUTH_ENV": "production", "AUTH_DATABASE_URL": "postgres://user:TOP_SECRET@db.example/auth?sslmode=verify-full", "AUTH_DEMO_EMAIL_VERIFICATION": "true", "AUTH_DEMO_TEST_EMAILS": "owner@example.test"}},
		{name: "empty demo allowlist", values: map[string]string{"AUTH_DEMO_EMAIL_VERIFICATION": "true"}},
		{name: "real email", values: map[string]string{"AUTH_DEMO_TEST_EMAILS": "owner@example.com"}},
		{name: "wildcard", values: map[string]string{"AUTH_DEMO_TEST_EMAILS": "*@example.test"}},
		{name: "display name", values: map[string]string{"AUTH_DEMO_TEST_EMAILS": "Owner <owner@example.test>"}},
		{name: "non ASCII", values: map[string]string{"AUTH_DEMO_TEST_EMAILS": "владелец@example.test"}},
		{name: "trailing empty entry", values: map[string]string{"AUTH_DEMO_TEST_EMAILS": "owner@example.test,"}},
		{name: "demo without RPC", values: map[string]string{"AUTH_GRPC_ADDRESS": "", "AUTH_GRPC_CERT_FILE": "", "AUTH_GRPC_KEY_FILE": "", "AUTH_GRPC_CA_FILE": "", "AUTH_DEMO_EMAIL_VERIFICATION": "true", "AUTH_DEMO_TEST_EMAILS": "owner@example.test"}},
		{name: "TLS without RPC", values: map[string]string{"AUTH_GRPC_ADDRESS": ""}},
		{name: "bad listener", values: map[string]string{"AUTH_GRPC_ADDRESS": "TOP_SECRET"}},
		{name: "unresolved hostname", values: map[string]string{"AUTH_GRPC_ADDRESS": "rpc.example.test:9092"}},
		{name: "bad port", values: map[string]string{"AUTH_GRPC_ADDRESS": "127.0.0.1:65536"}},
		{name: "missing certificate", values: map[string]string{"AUTH_GRPC_CERT_FILE": ""}},
		{name: "missing key", values: map[string]string{"AUTH_GRPC_KEY_FILE": ""}},
		{name: "missing CA", values: map[string]string{"AUTH_GRPC_CA_FILE": ""}},
		{name: "missing code HMAC", values: map[string]string{"AUTH_CODE_HMAC_KEY": ""}},
		{name: "missing CSRF HMAC", values: map[string]string{"AUTH_CSRF_HMAC_KEY": ""}},
		{name: "bad code HMAC", values: map[string]string{"AUTH_CODE_HMAC_KEY": "TOP_SECRET"}},
		{name: "bad CSRF HMAC", values: map[string]string{"AUTH_CSRF_HMAC_KEY": "TOP_SECRET"}},
		{name: "padded key", values: map[string]string{"AUTH_CODE_HMAC_KEY": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))}},
		{name: "short key", values: map[string]string{"AUTH_CODE_HMAC_KEY": base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 31))}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := rpcEnvironment()
			for name, value := range tc.values {
				env[name] = value
			}
			_, err := Load(func(key string) string { return env[key] })
			if err == nil {
				t.Fatal("unsafe RPC configuration accepted")
			}
			if strings.Contains(err.Error(), "TOP_SECRET") {
				t.Fatal("RPC configuration error exposed private input")
			}
		})
	}
}
