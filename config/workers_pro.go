//go:build !dev

package config

import (
	"os"
	"strconv"
)

// RefreshWorkers returns the number of concurrent workers to use.
// Prod default: 3. Override with REFRESH_WORKERS.
func RefreshWorkers() int {
	if v := os.Getenv("REFRESH_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 3
}
