package models

import "testing"

func TestGroupUserGamesByReleaseYear(t *testing.T) {
	games := []*UserGameWithGame{
		{Name: "Old", ReleaseYear: 2010},
		{Name: "New", ReleaseYear: 2020},
		{Name: "AlsoOld", ReleaseYear: 2010},
		{Name: "Unknown", ReleaseYear: 0},
	}

	groups := GroupUserGamesByReleaseYear(games)
	if len(groups) != 3 {
		t.Fatalf("len(groups) = %d, want 3", len(groups))
	}

	if groups[0].Label != "2020" || len(groups[0].Games) != 1 {
		t.Fatalf("first group = %+v, want 2020 with 1 game", groups[0])
	}
	if groups[1].Label != "2010" || len(groups[1].Games) != 2 {
		t.Fatalf("second group = %+v, want 2010 with 2 games", groups[1])
	}
	if groups[2].Label != "Unknown" || len(groups[2].Games) != 1 {
		t.Fatalf("third group = %+v, want Unknown with 1 game", groups[2])
	}
}

func TestGroupUserGamesByPlatform(t *testing.T) {
	games := []*UserGameWithGame{
		{Name: "A", Platform: "Steam"},
		{Name: "B", Platform: "Xbox"},
		{Name: "C", Platform: "Steam"},
	}

	groups := GroupUserGamesByPlatform(games)
	if len(groups) != 2 {
		t.Fatalf("len(groups) = %d, want 2", len(groups))
	}
	if groups[0].Platform != "Steam" || len(groups[0].Games) != 2 {
		t.Fatalf("first group = %+v, want Steam with 2 games", groups[0])
	}
	if groups[1].Platform != "Xbox" || len(groups[1].Games) != 1 {
		t.Fatalf("second group = %+v, want Xbox with 1 game", groups[1])
	}
}
