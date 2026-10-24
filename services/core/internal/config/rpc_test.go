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
	t.Run("ML pair", func(t *testing.T) {
		t.Setenv("CORE_ML_ADDRESS", "127.0.0.1:50054")
		t.Setenv("CORE_ML_SERVER_NAME", "")
		if _, err := config.LoadRPC(); err == nil {
			t.Fatal("ML without pinned hostname accepted")
		}
		t.Setenv("CORE_ML_SERVER_NAME", "saldo-ml")
		if cfg, err := config.LoadRPC(); err != nil || cfg.MLAddress != "127.0.0.1:50054" {
			t.Fatal(cfg, err)
		}
		t.Setenv("CORE_ML_ADDRESS", "")
		if _, err := config.LoadRPC(); err == nil {
			t.Fatal("ML hostname without address accepted")
		}
	})
	for name, value := range map[string]string{"CORE_GRPC_ADDRESS": ":50052", "CORE_AUTH_ADDRESS": "https://auth:443", "CORE_AUTH_TIMEOUT": "4s", "CORE_AUTH_SERVER_NAME": "", "CORE_MTLS_KEY_FILE": ""} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, value)
			if _, err := config.LoadRPC(); err == nil {
				t.Fatal("invalid RPC configuration accepted")
			}
		})
	}
}
