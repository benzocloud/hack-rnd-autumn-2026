package config

import (
	"testing"
	"time"
)

func TestCoreOptionalConfiguration(t *testing.T) {
	var c Config
	if err := c.loadCore(func(string) string { return "" }); err != nil || c.CoreAddress != "" || c.CoreTimeout != 2*time.Second {
		t.Fatal(c, err)
	}
	for _, tt := range []struct {
		name, address, server, timeout string
		valid                          bool
	}{
		{"configured", "core:50052", "core", "2s", true},
		{"local", "127.0.0.1:50052", "core.saldo.test", "50ms", true},
		{"missing server", "core:50052", "", "", false},
		{"missing address", "", "core", "", false},
		{"bad address", "https://core:50052", "core", "", false},
		{"bad port", "core:0", "core", "", false},
		{"long deadline", "core:50052", "core", "4s", false},
		{"invalid deadline", "core:50052", "core", "bad", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			env := map[string]string{"AUTH_CORE_ADDRESS": tt.address, "AUTH_CORE_SERVER_NAME": tt.server, "AUTH_CORE_TIMEOUT": tt.timeout}
			var cfg Config
			err := cfg.loadCore(func(k string) string { return env[k] })
			if (err == nil) != tt.valid {
				t.Fatal(err)
			}
		})
	}
}
