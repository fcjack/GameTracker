package gamename

import "testing"

func TestStripStorefrontSuffix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		title string
		want  string
	}{
		{"empty", "", ""},
		{"unchanged", "Halo Infinite", "Halo Infinite"},
		{"series suffix", "Forza Horizon 5 Xbox Series X|S", "Forza Horizon 5"},
		{"parenthetical", "Gears 5 (Xbox Series X|S)", "Gears 5"},
		{"xbox one", "Ori and the Blind Forest - Xbox One", "Ori and the Blind Forest"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := StripStorefrontSuffix(tt.title); got != tt.want {
				t.Errorf("StripStorefrontSuffix(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}

func TestStripEditionQualifier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		title string
		want  string
	}{
		{"game preview", "Grounded 2 (Game Preview)", "Grounded 2"},
		{"voidheart edition", "Hollow Knight: Voidheart Edition", "Hollow Knight"},
		{"unchanged", "Halo Infinite", "Halo Infinite"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := StripEditionQualifier(tt.title); got != tt.want {
				t.Errorf("StripEditionQualifier(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}
