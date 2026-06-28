package gamename

import (
	"strings"
	"unicode"
)

// Match reports whether two game titles likely refer to the same game.
func Match(a, b string) bool {
	left := normalize(a)
	right := normalize(b)
	if left == "" || right == "" {
		return false
	}
	if left == right {
		return true
	}
	return strings.Contains(left, right) || strings.Contains(right, left)
}

func normalize(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
