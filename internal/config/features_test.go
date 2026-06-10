package config

import "testing"

func TestParseBoolEnv(t *testing.T) {
	tests := []struct {
		value      string
		defaultVal bool
		want       bool
	}{
		{"", true, true},
		{"", false, false},
		{"true", false, true},
		{"TRUE", false, true},
		{" yes ", false, true},
		{"false", true, false},
		{"off", true, false},
		{"0", true, false},
		{"invalid", true, true},
		{"invalid", false, false},
	}

	for _, tt := range tests {
		if got := parseBoolEnv(tt.value, tt.defaultVal); got != tt.want {
			t.Errorf("parseBoolEnv(%q, %v) = %v, want %v", tt.value, tt.defaultVal, got, tt.want)
		}
	}
}
