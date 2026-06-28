package config

import (
	"os"
	"time"
)

const (
	envLibrarySyncEnabled  = "LIBRARY_SYNC_ENABLED"
	envLibrarySyncInterval = "LIBRARY_SYNC_INTERVAL"

	defaultLibrarySyncInterval = 6 * time.Hour
	// minLibrarySyncInterval guards against hammering external APIs (Steam/IGDB)
	// with an accidentally tiny interval.
	minLibrarySyncInterval = 15 * time.Minute
)

// LibrarySyncEnabled reports whether the background scheduler that periodically
// re-syncs linked libraries should run. Disabled by default (opt-in).
func LibrarySyncEnabled() bool {
	return parseBoolEnv(os.Getenv(envLibrarySyncEnabled), false)
}

// LibrarySyncInterval returns how often the background sync runs. Defaults to 6h
// when unset or unparseable, and is clamped to a safe minimum.
func LibrarySyncInterval() time.Duration {
	return parseDurationEnv(os.Getenv(envLibrarySyncInterval), defaultLibrarySyncInterval, minLibrarySyncInterval)
}

func parseDurationEnv(value string, defaultValue, minValue time.Duration) time.Duration {
	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 {
		d = defaultValue
	}
	if d < minValue {
		return minValue
	}
	return d
}
