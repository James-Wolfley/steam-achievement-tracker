//go:build dev

// config/schema_ttl_dev.go
package config

import (
	"os"
	"strconv"
	"time"
)

func SchemaTTL() time.Duration {
	if v := os.Getenv("SCHEMA_TTL_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return time.Duration(n) * time.Second
		}
	}
	// dev default: short TTL to catch changes quickly
	return 5 * time.Minute
}
