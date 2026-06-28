package models

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestRemoveSteamGamesFromLibrary(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	user, err := CreateUser(ctx, db, fmt.Sprintf("steam_clear_%d", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	cat, err := GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}

	steamGame, err := FindOrCreateGameBySteamAppID(ctx, db, 570, "Dota 2", "", cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGameBySteamAppID() error = %v", err)
	}
	manualGame, err := FindOrCreateGame(ctx, db, 99001, "Manual Game", "", 2020, []string{"PC"}, cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGame() error = %v", err)
	}

	if err := AddToLibrary(ctx, db, user.ID, steamGame.ID, "Steam", nil); err != nil {
		t.Fatalf("AddToLibrary(steam) error = %v", err)
	}
	if err := AddToLibrary(ctx, db, user.ID, manualGame.ID, "PC", nil); err != nil {
		t.Fatalf("AddToLibrary(manual) error = %v", err)
	}

	removed, err := RemoveSteamGamesFromLibrary(ctx, db, user.ID)
	if err != nil {
		t.Fatalf("RemoveSteamGamesFromLibrary() error = %v", err)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}

	games, err := ListUserGames(ctx, db, user.ID)
	if err != nil {
		t.Fatalf("ListUserGames() error = %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("library count = %d, want 1", len(games))
	}
	if games[0].Name != "Manual Game" || games[0].Platform != "PC" {
		t.Errorf("remaining game = %+v, want manual PC entry", games[0])
	}
}
