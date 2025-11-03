//go:build !dev

package config

import (
	"os"
	"strconv"
	"time"
)

// ThrottleWindow returns the refresh cooldown. Prod default: 60s.
// Override with THROTTLE_WINDOW_SECONDS.
func ThrottleWindow() time.Duration {
	if v := os.Getenv("THROTTLE_WINDOW_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return time.Duration(n) * time.Second
		}
	}
	return 60 * time.Second
}
