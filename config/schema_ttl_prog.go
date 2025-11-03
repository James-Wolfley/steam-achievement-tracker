//go:build !dev

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
	return time.Hour // 3600s
}
