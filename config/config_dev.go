//go:build dev

package config

import (
	"os"
	"strconv"
	"time"
)

// Dev default: 0s (no throttle). Still overrideable via env.
func ThrottleWindow() time.Duration {
	if v := os.Getenv("THROTTLE_WINDOW_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return time.Duration(n) * time.Second
		}
	}
	return 0
}
