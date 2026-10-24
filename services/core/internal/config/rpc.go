package config

import (
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

var errRPCConfiguration = errors.New("core RPC requires explicit address, Auth address/server name and mTLS CA/certificate/key files")

type RPC struct {
	Address, AuthAddress, AuthServerName string
	CAFile, CertificateFile, KeyFile     string
	AuthTimeout                          time.Duration
}

// LoadRPC is separate from foundation config: only a listening server needs it.
// There is no plaintext/demo switch for service authentication.
func LoadRPC() (RPC, error) {
	cfg := RPC{Address: os.Getenv("CORE_GRPC_ADDRESS"), AuthAddress: os.Getenv("CORE_AUTH_ADDRESS"), AuthServerName: os.Getenv("CORE_AUTH_SERVER_NAME"),
		CAFile: os.Getenv("CORE_MTLS_CA_FILE"), CertificateFile: os.Getenv("CORE_MTLS_CERT_FILE"), KeyFile: os.Getenv("CORE_MTLS_KEY_FILE"), AuthTimeout: 3 * time.Second}
	if value, ok := os.LookupEnv("CORE_AUTH_TIMEOUT"); ok {
		timeout, err := time.ParseDuration(value)
		if err != nil {
			return RPC{}, errRPCConfiguration
		}
		cfg.AuthTimeout = timeout
	}
	return cfg, cfg.Validate()
}

func (cfg RPC) Validate() error {
	if !validAddress(cfg.Address) || !validAddress(cfg.AuthAddress) || strings.TrimSpace(cfg.AuthServerName) == "" ||
		strings.TrimSpace(cfg.CAFile) == "" || strings.TrimSpace(cfg.CertificateFile) == "" || strings.TrimSpace(cfg.KeyFile) == "" ||
		cfg.AuthTimeout <= 0 || cfg.AuthTimeout > 3*time.Second {
		return errRPCConfiguration
	}
	return nil
}

func validAddress(address string) bool {
	host, port, err := net.SplitHostPort(address)
	number, numberErr := strconv.ParseUint(port, 10, 16)
	return err == nil && numberErr == nil && number > 0 && host != "" && !strings.ContainsAny(host, " \t\r\n/?#@\\")
}
