package i18n

// FormatPlaytime renders a minute count as "X H and Y minutes" for library cards.
func FormatPlaytime(locale string, minutes int) string {
	hours := minutes / 60
	mins := minutes % 60

	key := "game.playtime_hours_minutes"
	if mins == 1 {
		key = "game.playtime_hours_minute"
	}
	return T(locale, key, hours, mins)
}
