package models

import (
	"context"
	"testing"
)

func TestAddToLibrary_storesPlaytime(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	defer db.Close()

	user, err := CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	cat, err := GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}
	game, err := FindOrCreateGameBySteamAppID(ctx, db, 570, "Dota 2", "https://cdn.example.com/dota.jpg", cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGameBySteamAppID() error = %v", err)
	}

	playtime := 150
	if err := AddToLibrary(ctx, db, user.ID, game.ID, "Steam", &playtime); err != nil {
		t.Fatalf("AddToLibrary() error = %v", err)
	}

	got, err := GetUserGame(ctx, db, user.ID, game.ID)
	if err != nil {
		t.Fatalf("GetUserGame() error = %v", err)
	}
	if got.PlaytimeMinutes == nil || *got.PlaytimeMinutes != 150 {
		t.Fatalf("PlaytimeMinutes = %v, want 150", got.PlaytimeMinutes)
	}
}

func TestUpdatePlaytimeBySteamAppID(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	defer db.Close()

	user, err := CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	cat, err := GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}
	game, err := FindOrCreateGameBySteamAppID(ctx, db, 570, "Dota 2", "https://cdn.example.com/dota.jpg", cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGameBySteamAppID() error = %v", err)
	}
	if err := AddToLibrary(ctx, db, user.ID, game.ID, "Steam", nil); err != nil {
		t.Fatalf("AddToLibrary() error = %v", err)
	}

	if err := UpdatePlaytimeBySteamAppID(ctx, db, user.ID, "Steam", 570, 8520); err != nil {
		t.Fatalf("UpdatePlaytimeBySteamAppID() error = %v", err)
	}

	got, err := GetUserGame(ctx, db, user.ID, game.ID)
	if err != nil {
		t.Fatalf("GetUserGame() error = %v", err)
	}
	if got.PlaytimeMinutes == nil || *got.PlaytimeMinutes != 8520 {
		t.Fatalf("PlaytimeMinutes = %v, want 8520", got.PlaytimeMinutes)
	}
}
