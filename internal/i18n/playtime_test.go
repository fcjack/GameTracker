package i18n

import "testing"

func TestFormatPlaytime(t *testing.T) {
	t.Parallel()
	tests := []struct {
		locale  string
		minutes int
		want    string
	}{
		{LocaleEN, 0, "0 H and 0 minutes"},
		{LocaleEN, 45, "0 H and 45 minutes"},
		{LocaleEN, 60, "1 H and 0 minutes"},
		{LocaleEN, 61, "1 H and 1 minute"},
		{LocaleEN, 150, "2 H and 30 minutes"},
		{LocaleEN, 8520, "142 H and 0 minutes"},
		{LocalePTBR, 45, "0 h e 45 minutos"},
		{LocalePTBR, 61, "1 h e 1 minuto"},
		{LocalePTBR, 150, "2 h e 30 minutos"},
	}
	for _, tt := range tests {
		if got := FormatPlaytime(tt.locale, tt.minutes); got != tt.want {
			t.Errorf("FormatPlaytime(%q, %d) = %q, want %q", tt.locale, tt.minutes, got, tt.want)
		}
	}
}
