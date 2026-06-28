package i18n

import "testing"

func TestFormatPlaytime(t *testing.T) {
	t.Parallel()
	tests := []struct {
		locale  string
		minutes int
		want    string
	}{
		{LocaleEN, 45, "45 min"},
		{LocaleEN, 60, "1 hr"},
		{LocaleEN, 150, "2.5 hrs"},
		{LocaleEN, 8520, "142 hrs"},
		{LocalePTBR, 45, "45 min"},
		{LocalePTBR, 150, "2.5 h"},
	}
	for _, tt := range tests {
		if got := FormatPlaytime(tt.locale, tt.minutes); got != tt.want {
			t.Errorf("FormatPlaytime(%q, %d) = %q, want %q", tt.locale, tt.minutes, got, tt.want)
		}
	}
}
