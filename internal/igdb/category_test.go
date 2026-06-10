package igdb

import "testing"

func TestIsMainGame(t *testing.T) {
	if !IsMainGame(0) {
		t.Error("category 0 should be a main game")
	}
	for _, cat := range []int{1, 2, 6, 14} {
		if IsMainGame(cat) {
			t.Errorf("category %d should not be a main game", cat)
		}
	}
}
