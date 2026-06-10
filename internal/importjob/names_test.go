package importjob

import "testing"

func TestNamesMatch(t *testing.T) {
	tests := []struct {
		steam string
		igdb  string
		want  bool
	}{
		{"Dota 2", "Dota 2", true},
		{"Counter-Strike 2", "Counter-Strike 2", true},
		{"Grand Theft Auto V", "Grand Theft Auto V", true},
		{"Hollow Knight", "Hollow Knight: Silksong", true},
		{"Totally Different Game", "Another Title", false},
		{"", "Dota 2", false},
	}

	for _, tt := range tests {
		if got := namesMatch(tt.steam, tt.igdb); got != tt.want {
			t.Errorf("namesMatch(%q, %q) = %v, want %v", tt.steam, tt.igdb, got, tt.want)
		}
	}
}
