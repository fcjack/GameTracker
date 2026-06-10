package config

import (
	"os"
	"strings"
)

const envSignInEnabled = "SIGN_IN_ENABLED"

// SignInEnabled reports whether sign-in and registration are allowed.
// Defaults to true when SIGN_IN_ENABLED is unset.
func SignInEnabled() bool {
	return parseBoolEnv(os.Getenv(envSignInEnabled), true)
}

func parseBoolEnv(value string, defaultValue bool) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return defaultValue
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return defaultValue
	}
}
