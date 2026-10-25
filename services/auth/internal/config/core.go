package config

import (
	"errors"
	"net"
	"strconv"
	"strings"
	"time"
)

func (c *Config) loadCore(getenv func(string) string) error {
	c.CoreAddress = getenv("AUTH_CORE_ADDRESS")
	c.CoreServerName = getenv("AUTH_CORE_SERVER_NAME")
	c.CoreTimeout = 2 * time.Second
	if value := getenv("AUTH_CORE_TIMEOUT"); value != "" {
		timeout, err := time.ParseDuration(value)
		if err != nil || timeout < 50*time.Millisecond || timeout > 3*time.Second {
			return errors.New("AUTH_CORE_TIMEOUT must be between 50ms and 3s")
		}
		c.CoreTimeout = timeout
	}
	if c.CoreAddress == "" {
		if c.CoreServerName != "" {
			return errors.New("AUTH_CORE_ADDRESS is required with Core server name")
		}
		return nil
	}
	host, portText, err := net.SplitHostPort(c.CoreAddress)
	if err != nil || host == "" || strings.ContainsAny(host, " /?#@\t\n\r") {
		return errors.New("AUTH_CORE_ADDRESS requires host and port")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("AUTH_CORE_ADDRESS has invalid port")
	}
	if strings.TrimSpace(c.CoreServerName) == "" {
		return errors.New("AUTH_CORE_SERVER_NAME is required")
	}
	return nil
}
