package config

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"strconv"
	"strings"
)

// HMAC key accessors return independent copies; the config never renders key material.
func (c Config) CodeHMACKey() []byte { return bytes.Clone(c.codeHMACKey) }

func (c Config) CSRFHMACKey() []byte { return bytes.Clone(c.csrfHMACKey) }

func (c *Config) loadRPC(getenv func(string) string) error {
	c.GRPCAddress = getenv("AUTH_GRPC_ADDRESS")
	c.GRPCCertFile = getenv("AUTH_GRPC_CERT_FILE")
	c.GRPCKeyFile = getenv("AUTH_GRPC_KEY_FILE")
	c.GRPCCAFile = getenv("AUTH_GRPC_CA_FILE")
	switch getenv("AUTH_DEMO_EMAIL_VERIFICATION") {
	case "", "false":
		c.DemoEmailVerification = false
	case "true":
		c.DemoEmailVerification = true
	default:
		return errors.New("AUTH_DEMO_EMAIL_VERIFICATION must be true or false")
	}
	if value := getenv("AUTH_DEMO_TEST_EMAILS"); value != "" {
		for _, address := range strings.Split(value, ",") {
			c.DemoTestEmails = append(c.DemoTestEmails, strings.ToLower(strings.TrimSpace(address)))
		}
	}
	var err error
	c.codeHMACKey, err = decodeHMACKey(getenv("AUTH_CODE_HMAC_KEY"))
	if err != nil {
		return fmt.Errorf("AUTH_CODE_HMAC_KEY: %w", err)
	}
	c.csrfHMACKey, err = decodeHMACKey(getenv("AUTH_CSRF_HMAC_KEY"))
	if err != nil {
		return fmt.Errorf("AUTH_CSRF_HMAC_KEY: %w", err)
	}
	return validateRPC(*c)
}

func decodeHMACKey(value string) ([]byte, error) {
	if value == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if len(value) != 43 || err != nil || len(decoded) != 32 {
		return nil, errors.New("must encode exactly 32 bytes as unpadded base64url")
	}
	return decoded, nil
}

func validateRPC(c Config) error {
	if err := validateDemo(c); err != nil {
		return err
	}
	if c.GRPCAddress == "" {
		if c.GRPCCertFile != "" || c.GRPCKeyFile != "" || c.GRPCCAFile != "" {
			return errors.New("AUTH_GRPC_ADDRESS is required with gRPC TLS settings")
		}
		return nil
	}
	host, portText, err := net.SplitHostPort(c.GRPCAddress)
	if err != nil || (host != "localhost" && net.ParseIP(host) == nil) {
		return errors.New("AUTH_GRPC_ADDRESS requires a literal IP or localhost and port")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 0 || port > 65535 || (port == 0 && c.Environment != "local") {
		return errors.New("AUTH_GRPC_ADDRESS has an invalid port")
	}
	if strings.TrimSpace(c.GRPCCertFile) == "" || strings.TrimSpace(c.GRPCKeyFile) == "" || strings.TrimSpace(c.GRPCCAFile) == "" {
		return errors.New("gRPC requires AUTH_GRPC_CERT_FILE, AUTH_GRPC_KEY_FILE and AUTH_GRPC_CA_FILE")
	}
	if len(c.codeHMACKey) != 32 || len(c.csrfHMACKey) != 32 {
		return errors.New("gRPC requires AUTH_CODE_HMAC_KEY and AUTH_CSRF_HMAC_KEY settings")
	}
	return nil
}

func validateDemo(c Config) error {
	for _, value := range c.DemoTestEmails {
		address, err := mail.ParseAddress(value)
		at := strings.LastIndexByte(value, '@')
		if err != nil || address.Address != value || at < 1 || len(value) > 254 || strings.ContainsAny(value, "*?") ||
			!strings.HasSuffix(value[at+1:], ".test") || len(value[at+1:]) <= len(".test") {
			return errors.New("AUTH_DEMO_TEST_EMAILS requires exact synthetic .test email addresses")
		}
		for _, character := range value {
			if character < '!' || character > '~' {
				return errors.New("AUTH_DEMO_TEST_EMAILS requires ASCII email addresses")
			}
		}
	}
	if !c.DemoEmailVerification {
		return nil
	}
	if c.Environment != "local" {
		return errors.New("demo email verification requires AUTH_ENV=local")
	}
	if len(c.DemoTestEmails) == 0 {
		return errors.New("demo email verification requires AUTH_DEMO_TEST_EMAILS")
	}
	if c.GRPCAddress == "" {
		return errors.New("demo email verification requires AUTH_GRPC_ADDRESS")
	}
	return nil
}
