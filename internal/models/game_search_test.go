package models

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestSearchUserGames_matchesByName(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	user, err := CreateUser(ctx, db, fmt.Sprintf("search_name_%d", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	cat, err := GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}

	for i, name := range []string{"Elden Ring", "Dark Souls III", "Bloodborne"} {
		game, err := FindOrCreateGame(ctx, db, int64(91000+i), name, "", 2015+i, []string{"PC"}, cat.ID)
		if err != nil {
			t.Fatalf("FindOrCreateGame() error = %v", err)
		}
		if err := AddToLibrary(ctx, db, user.ID, game.ID, "PC"); err != nil {
			t.Fatalf("AddToLibrary() error = %v", err)
		}
	}

	results, err := SearchUserGames(ctx, db, user.ID, "souls", 10)
	if err != nil {
		t.Fatalf("SearchUserGames() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("SearchUserGames() returned %d games, want 1", len(results))
	}
	if results[0].Name != "Dark Souls III" {
		t.Errorf("result name = %q, want Dark Souls III", results[0].Name)
	}
}

func TestSearchUserGames_matchesByPlatform(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	user, err := CreateUser(ctx, db, fmt.Sprintf("search_platform_%d", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	cat, err := GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}

	game, err := FindOrCreateGame(ctx, db, 92000, "Hollow Knight", "", 2017, []string{"PC", "Nintendo Switch"}, cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGame() error = %v", err)
	}
	if err := AddToLibrary(ctx, db, user.ID, game.ID, "Nintendo Switch"); err != nil {
		t.Fatalf("AddToLibrary() error = %v", err)
	}

	results, err := SearchUserGames(ctx, db, user.ID, "switch", 10)
	if err != nil {
		t.Fatalf("SearchUserGames() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("SearchUserGames() returned %d games, want 1", len(results))
	}
	if results[0].Platform != "Nintendo Switch" {
		t.Errorf("result platform = %q, want Nintendo Switch", results[0].Platform)
	}
}

func TestSearchUserGames_caseInsensitive(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	user, err := CreateUser(ctx, db, fmt.Sprintf("search_case_%d", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	cat, err := GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}

	game, err := FindOrCreateGame(ctx, db, 93000, "The Witcher 3", "", 2015, []string{"PC"}, cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGame() error = %v", err)
	}
	if err := AddToLibrary(ctx, db, user.ID, game.ID, "PC"); err != nil {
		t.Fatalf("AddToLibrary() error = %v", err)
	}

	results, err := SearchUserGames(ctx, db, user.ID, "WITCHER", 10)
	if err != nil {
		t.Fatalf("SearchUserGames() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("SearchUserGames() returned %d games, want 1", len(results))
	}
}

func TestSearchUserGames_excludesOtherUsers(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	owner, err := CreateUser(ctx, db, fmt.Sprintf("search_owner_%d", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	other, err := CreateUser(ctx, db, fmt.Sprintf("search_other_%d", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	cat, err := GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}

	game, err := FindOrCreateGame(ctx, db, 94000, "Private Library Game", "", 2020, []string{"PC"}, cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGame() error = %v", err)
	}
	if err := AddToLibrary(ctx, db, owner.ID, game.ID, "PC"); err != nil {
		t.Fatalf("AddToLibrary() error = %v", err)
	}

	results, err := SearchUserGames(ctx, db, other.ID, "private", 10)
	if err != nil {
		t.Fatalf("SearchUserGames() error = %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("SearchUserGames() returned %d games, want 0", len(results))
	}
}

func TestSearchUserGames_noMatch(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	user, err := CreateUser(ctx, db, fmt.Sprintf("search_none_%d", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	cat, err := GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}

	game, err := FindOrCreateGame(ctx, db, 95000, "Celeste", "", 2018, []string{"PC"}, cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGame() error = %v", err)
	}
	if err := AddToLibrary(ctx, db, user.ID, game.ID, "PC"); err != nil {
		t.Fatalf("AddToLibrary() error = %v", err)
	}

	results, err := SearchUserGames(ctx, db, user.ID, "zelda", 10)
	if err != nil {
		t.Fatalf("SearchUserGames() error = %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("SearchUserGames() returned %d games, want 0", len(results))
	}
}
