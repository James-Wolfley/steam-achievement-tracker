//go:build dev

package config

import (
	"os"
	"strconv"
)

// Dev default: 3 (still reasonable for Steam). Override with REFRESH_WORKERS.
func RefreshWorkers() int {
	if v := os.Getenv("REFRESH_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 3
}
