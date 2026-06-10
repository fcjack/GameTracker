package config

import (
	"os"
	"strings"
)

const envSignInEnabled = "SIGN_IN_ENABLED"

// RegistrationEnabled reports whether new account registration is allowed.
// Defaults to true when SIGN_IN_ENABLED is unset. When false, existing users can still sign in.
func RegistrationEnabled() bool {
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
