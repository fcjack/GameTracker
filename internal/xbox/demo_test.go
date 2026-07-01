package xbox

import "testing"

func TestIsDemoTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		title string
		want  bool
	}{
		{"empty", "", false},
		{"demo suffix", "Halo Infinite Demo", true},
		{"demo suffix uppercase", "FORZA HORIZON 5 DEMO", true},
		{"brackets", "Game [Demo]", true},
		{"parentheses", "Game (Demo)", true},
		{"demo only", "Demo", true},
		{"full game", "Halo Infinite", false},
		{"false positive democracy", "Democracy 4", false},
		{"false positive demon", "Demon's Souls", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsDemoTitle(tt.title); got != tt.want {
				t.Errorf("IsDemoTitle(%q) = %v, want %v", tt.title, got, tt.want)
			}
		})
	}
}
