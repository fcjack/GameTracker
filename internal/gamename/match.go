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
	return canonicalizeNumerals(strings.Join(strings.Fields(b.String()), " "))
}

var romanToArabic = map[string]string{
	"i": "1", "ii": "2", "iii": "3", "iv": "4", "v": "5",
	"vi": "6", "vii": "7", "viii": "8", "ix": "9",
}

func canonicalizeNumerals(s string) string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return s
	}
	last := words[len(words)-1]
	if arabic, ok := romanToArabic[last]; ok {
		words[len(words)-1] = arabic
		return strings.Join(words, " ")
	}
	return s
}
