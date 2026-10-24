package postgres

import (
	"errors"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/config"
)

var errInvalidConfig = errors.New("invalid postgres pool configuration")

func validatePostgres(c config.Postgres) error {
	if c.Host == "" || c.Port == 0 || c.User == "" || c.Password == "" || c.Database == "" || c.SSLMode == "" ||
		c.MaxConns < 1 || c.MaxIdleConns < 0 || c.MaxIdleConns > c.MaxConns || c.ConnectTimeout <= 0 || c.QueryTimeout <= 0 {
		return errInvalidConfig
	}
	return nil
}
