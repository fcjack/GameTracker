package config

import (
	"testing"
	"time"
)

func TestParseDurationEnv(t *testing.T) {
	t.Parallel()
	const def = 6 * time.Hour
	const min = 15 * time.Minute

	tests := []struct {
		value string
		want  time.Duration
	}{
		{"", def},
		{"invalid", def},
		{"0", def},
		{"-5m", def},
		{"6h", 6 * time.Hour},
		{"30m", 30 * time.Minute},
		{"12h", 12 * time.Hour},
		{"1m", min},  // clamped up to minimum
		{"14m", min}, // clamped up to minimum
		{"15m", min},
	}

	for _, tt := range tests {
		if got := parseDurationEnv(tt.value, def, min); got != tt.want {
			t.Errorf("parseDurationEnv(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

func TestLibrarySyncEnabledDefault(t *testing.T) {
	t.Setenv(envLibrarySyncEnabled, "")
	if LibrarySyncEnabled() {
		t.Error("LibrarySyncEnabled() = true, want false by default")
	}
	t.Setenv(envLibrarySyncEnabled, "true")
	if !LibrarySyncEnabled() {
		t.Error("LibrarySyncEnabled() = false, want true when set")
	}
}

func TestLibrarySyncIntervalDefault(t *testing.T) {
	t.Setenv(envLibrarySyncInterval, "")
	if got := LibrarySyncInterval(); got != defaultLibrarySyncInterval {
		t.Errorf("LibrarySyncInterval() = %v, want %v", got, defaultLibrarySyncInterval)
	}
	t.Setenv(envLibrarySyncInterval, "2h")
	if got := LibrarySyncInterval(); got != 2*time.Hour {
		t.Errorf("LibrarySyncInterval() = %v, want 2h", got)
	}
}
