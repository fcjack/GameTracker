package gamename

import "testing"

func TestMatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		a    string
		b    string
		want bool
	}{
		{"Dota 2", "Dota 2", true},
		{"Counter-Strike 2", "Counter-Strike 2", true},
		{"Grand Theft Auto V", "Grand Theft Auto V", true},
		{"Hollow Knight", "Hollow Knight: Silksong", true},
		{"Totally Different Game", "Another Title", false},
		{"", "Dota 2", false},
	}

	for _, tt := range tests {
		if got := Match(tt.a, tt.b); got != tt.want {
			t.Errorf("Match(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}
