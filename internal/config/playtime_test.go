package config

import "testing"

func TestPlaytimeWorkerCount(t *testing.T) {
	t.Setenv(envPlaytimeWorkerCount, "5")
	if got := PlaytimeWorkerCount(); got != 5 {
		t.Fatalf("PlaytimeWorkerCount() = %d, want 5", got)
	}
}

func TestPlaytimeWorkerCountClamped(t *testing.T) {
	t.Setenv(envPlaytimeWorkerCount, "99")
	if got := PlaytimeWorkerCount(); got != maxPlaytimeWorkerCount {
		t.Fatalf("PlaytimeWorkerCount() = %d, want %d", got, maxPlaytimeWorkerCount)
	}
}
