package importjob

import (
	"strings"
	"unicode"
)

// namesMatch reports whether a Steam title and IGDB title likely refer to the same game.
func namesMatch(steamName, igdbName string) bool {
	a := normalizeGameName(steamName)
	b := normalizeGameName(igdbName)
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	return strings.Contains(a, b) || strings.Contains(b, a)
}

func normalizeGameName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
