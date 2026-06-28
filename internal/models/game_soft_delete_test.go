package models

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestRemoveFromLibrary_softDeletes(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	user, err := CreateUser(ctx, db, fmt.Sprintf("soft_del_%d", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	cat, err := GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}

	game, err := FindOrCreateGameBySteamAppID(ctx, db, 570, "Dota 2", "", cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGameBySteamAppID() error = %v", err)
	}
	if err := AddToLibrary(ctx, db, user.ID, game.ID, "Steam", nil); err != nil {
		t.Fatalf("AddToLibrary() error = %v", err)
	}

	if err := RemoveFromLibrary(ctx, db, user.ID, game.ID); err != nil {
		t.Fatalf("RemoveFromLibrary() error = %v", err)
	}

	games, err := ListUserGames(ctx, db, user.ID)
	if err != nil {
		t.Fatalf("ListUserGames() error = %v", err)
	}
	if len(games) != 0 {
		t.Fatalf("ListUserGames() returned %d games, want 0 after soft delete", len(games))
	}

	inLibrary, err := IsInLibrary(ctx, db, user.ID, game.ID)
	if err != nil {
		t.Fatalf("IsInLibrary() error = %v", err)
	}
	if inLibrary {
		t.Fatal("IsInLibrary() = true, want false for soft-deleted game")
	}

	exists, err := LibraryEntryExists(ctx, db, user.ID, game.ID)
	if err != nil {
		t.Fatalf("LibraryEntryExists() error = %v", err)
	}
	if !exists {
		t.Fatal("LibraryEntryExists() = false, want true to block Steam re-import")
	}

	ids, err := ListImportedSteamAppIDs(ctx, db, user.ID, "Steam")
	if err != nil {
		t.Fatalf("ListImportedSteamAppIDs() error = %v", err)
	}
	if _, ok := ids[570]; !ok {
		t.Fatal("ListImportedSteamAppIDs() missing soft-deleted Steam app")
	}
}

func TestAddToLibrary_restoresSoftDeleted(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	user, err := CreateUser(ctx, db, fmt.Sprintf("restore_%d", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	cat, err := GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}

	game, err := FindOrCreateGame(ctx, db, 92001, "Restorable Game", "", 2020, []string{"PC"}, cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGame() error = %v", err)
	}
	if err := AddToLibrary(ctx, db, user.ID, game.ID, "PC", nil); err != nil {
		t.Fatalf("AddToLibrary() error = %v", err)
	}
	if err := RemoveFromLibrary(ctx, db, user.ID, game.ID); err != nil {
		t.Fatalf("RemoveFromLibrary() error = %v", err)
	}

	if err := AddToLibrary(ctx, db, user.ID, game.ID, "PC", nil); err != nil {
		t.Fatalf("AddToLibrary() restore error = %v", err)
	}

	inLibrary, err := IsInLibrary(ctx, db, user.ID, game.ID)
	if err != nil {
		t.Fatalf("IsInLibrary() error = %v", err)
	}
	if !inLibrary {
		t.Fatal("IsInLibrary() = false after manual restore, want true")
	}

	games, err := ListUserGames(ctx, db, user.ID)
	if err != nil {
		t.Fatalf("ListUserGames() error = %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("ListUserGames() returned %d games, want 1 after restore", len(games))
	}
}
