package i18n

import "math"

// FormatPlaytime renders a minute count for library cards (hours when >= 60 min).
func FormatPlaytime(locale string, minutes int) string {
	if minutes < 60 {
		return T(locale, "game.playtime_minutes", minutes)
	}
	if minutes%60 == 0 {
		hours := minutes / 60
		key := "game.playtime_hours"
		if hours != 1 {
			key = "game.playtime_hours_plural"
		}
		return T(locale, key, hours)
	}
	hours := math.Round(float64(minutes)/60*10) / 10
	return T(locale, "game.playtime_hours_decimal", hours)
}
