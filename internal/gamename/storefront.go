package gamename

import "strings"

var storefrontSuffixes = []string{
	" (xbox series x|s)",
	" (xbox series xs)",
	" (xbox one)",
	" (xbox 360)",
	" - xbox series x|s",
	" - xbox one",
	" - xbox 360",
	" for xbox",
	" xbox series x|s",
	" xbox one",
	" xbox 360",
}

// StripStorefrontSuffix removes common Xbox storefront qualifiers from a title.
func StripStorefrontSuffix(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return name
	}

	lower := strings.ToLower(name)
	for {
		changed := false
		for _, suffix := range storefrontSuffixes {
			if strings.HasSuffix(lower, suffix) {
				name = strings.TrimSpace(name[:len(name)-len(suffix)])
				lower = strings.ToLower(name)
				changed = true
				break
			}
		}
		if !changed {
			break
		}
	}
	return name
}

// StripEditionQualifier removes Xbox-specific edition and preview suffixes for IGDB search.
func StripEditionQualifier(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return name
	}

	lower := strings.ToLower(name)
	for _, marker := range []string{"(game preview)", "(preview)"} {
		if idx := strings.Index(lower, marker); idx >= 0 {
			name = strings.TrimSpace(name[:idx])
			lower = strings.ToLower(name)
		}
	}

	if strings.Contains(lower, " edition") {
		if idx := strings.LastIndex(name, ":"); idx > 0 {
			name = strings.TrimSpace(name[:idx])
		}
	}

	return name
}
