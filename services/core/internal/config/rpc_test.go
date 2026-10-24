package config_test

import (
	"testing"
	"time"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/config"
)

func TestLoadRPC(t *testing.T) {
	for key, value := range map[string]string{"CORE_GRPC_ADDRESS": "127.0.0.1:50052", "CORE_AUTH_ADDRESS": "127.0.0.1:50053", "CORE_AUTH_SERVER_NAME": "saldo-auth",
		"CORE_MTLS_CA_FILE": "ca.pem", "CORE_MTLS_CERT_FILE": "cert.pem", "CORE_MTLS_KEY_FILE": "key.pem", "CORE_AUTH_TIMEOUT": "3s"} {
		t.Setenv(key, value)
	}
	cfg, err := config.LoadRPC()
	if err != nil || cfg.AuthTimeout != 3*time.Second {
		t.Fatal(cfg, err)
	}
	for name, value := range map[string]string{"CORE_GRPC_ADDRESS": ":50052", "CORE_AUTH_ADDRESS": "https://auth:443", "CORE_AUTH_TIMEOUT": "4s", "CORE_AUTH_SERVER_NAME": "", "CORE_MTLS_KEY_FILE": ""} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, value)
			if _, err := config.LoadRPC(); err == nil {
				t.Fatal("invalid RPC configuration accepted")
			}
		})
	}
}
