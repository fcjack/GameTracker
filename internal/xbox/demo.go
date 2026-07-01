package xbox

import "strings"

// IsDemoTitle reports whether an Xbox title name refers to a demo rather than a full game.
func IsDemoTitle(name string) bool {
	upper := strings.ToUpper(strings.TrimSpace(name))
	if upper == "" {
		return false
	}
	if upper == "DEMO" {
		return true
	}
	if strings.HasSuffix(upper, " DEMO") {
		return true
	}
	return strings.Contains(upper, "(DEMO)") || strings.Contains(upper, "[DEMO]")
}

func isDemoProgram(value string) bool {
	upper := strings.ToUpper(strings.TrimSpace(value))
	if upper == "" {
		return false
	}
	return upper == "DEMO" || strings.Contains(upper, "DEMO")
}

func (entry titleHistoryEntry) isDemo() bool {
	name := entry.Name
	if name == "" {
		name = entry.Detail.Name
	}
	if IsDemoTitle(name) {
		return true
	}
	for _, value := range entry.Detail.Programs {
		if isDemoProgram(value) {
			return true
		}
	}
	for _, value := range entry.Detail.UserPrograms {
		if isDemoProgram(value) {
			return true
		}
	}
	return false
}
