package models

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestFindOrCreateGameByXboxTitleID(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	cat, err := GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}

	coverURL := "https://cdn.example.com/xbox/123.jpg"
	game, err := FindOrCreateGameByXboxTitleID(ctx, db, 123456789, "Xbox Only", coverURL, cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGameByXboxTitleID() error = %v", err)
	}
	if game.IGDBId != nil {
		t.Errorf("igdb_id = %v, want nil", game.IGDBId)
	}
	if game.XboxTitleID == nil || *game.XboxTitleID != 123456789 {
		t.Errorf("xbox_title_id = %v, want 123456789", game.XboxTitleID)
	}
	if game.CoverURL != coverURL {
		t.Errorf("cover_url = %q, want %q", game.CoverURL, coverURL)
	}

	again, err := FindOrCreateGameByXboxTitleID(ctx, db, 123456789, "Xbox Only Renamed", coverURL, cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGameByXboxTitleID() second call error = %v", err)
	}
	if again.ID != game.ID {
		t.Errorf("id = %d, want same row %d", again.ID, game.ID)
	}
	if again.Name != "Xbox Only Renamed" {
		t.Errorf("name = %q, want updated name", again.Name)
	}
}

func TestLinkIGDBToXboxGame(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	cat, err := GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}

	xboxOnly, err := FindOrCreateGameByXboxTitleID(ctx, db, 456789012, "Linkable Xbox Game", "https://cdn.example.com/xbox.jpg", cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGameByXboxTitleID() error = %v", err)
	}

	linked, err := LinkIGDBToXboxGame(
		ctx, db, 456789012, 98766,
		"Linkable Xbox Game IGDB", "https://cdn.example.com/igdb.jpg",
		2020, []string{"Xbox One"}, cat.ID,
	)
	if err != nil {
		t.Fatalf("LinkIGDBToXboxGame() error = %v", err)
	}
	if linked.ID != xboxOnly.ID {
		t.Errorf("id = %d, want %d", linked.ID, xboxOnly.ID)
	}
	if linked.IGDBId == nil || *linked.IGDBId != 98766 {
		t.Errorf("igdb_id = %v, want 98766", linked.IGDBId)
	}

	got, err := GetGameByXboxTitleID(ctx, db, 456789012)
	if err != nil {
		t.Fatalf("GetGameByXboxTitleID() error = %v", err)
	}
	if got.IGDBId == nil || *got.IGDBId != 98766 {
		t.Errorf("stored igdb_id = %v, want 98766", got.IGDBId)
	}
}

func TestResolveGameForXboxImportMergesDuplicateRows(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	cat, err := GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}

	suffix := time.Now().UnixNano() % 1000000
	igdbID := int64(9977000000 + suffix)
	xboxTitleID := int(9977001000 + int(suffix))

	igdbOnly, err := FindOrCreateGameWithXboxTitleID(
		ctx, db, igdbID, nil,
		"Merged Xbox Game IGDB", "https://cdn.example.com/igdb.jpg",
		2019, []string{"Xbox Series X|S"}, cat.ID,
	)
	if err != nil {
		t.Fatalf("FindOrCreateGameWithXboxTitleID() igdb row error = %v", err)
	}

	xboxOnly, err := FindOrCreateGameByXboxTitleID(
		ctx, db, xboxTitleID, "Merged Xbox Game", "https://cdn.example.com/xbox.jpg", cat.ID,
	)
	if err != nil {
		t.Fatalf("FindOrCreateGameByXboxTitleID() xbox row error = %v", err)
	}
	if xboxOnly.ID == igdbOnly.ID {
		t.Fatal("expected separate rows before merge")
	}

	resolved, err := ResolveGameForXboxImport(
		ctx, db, xboxTitleID, igdbID,
		"Merged Xbox Game Final", "https://cdn.example.com/final.jpg",
		2019, []string{"Xbox Series X|S", "Xbox One"}, cat.ID,
	)
	if err != nil {
		t.Fatalf("ResolveGameForXboxImport() error = %v", err)
	}
	if resolved.ID != igdbOnly.ID {
		t.Errorf("resolved id = %d, want canonical igdb row %d", resolved.ID, igdbOnly.ID)
	}
	if resolved.XboxTitleID == nil || *resolved.XboxTitleID != xboxTitleID {
		t.Errorf("xbox_title_id = %v, want %d", resolved.XboxTitleID, xboxTitleID)
	}
	if resolved.IGDBId == nil || *resolved.IGDBId != igdbID {
		t.Errorf("igdb_id = %v, want %d", resolved.IGDBId, igdbID)
	}

	byXbox, err := GetGameByXboxTitleID(ctx, db, xboxTitleID)
	if err != nil {
		t.Fatalf("GetGameByXboxTitleID() after merge error = %v", err)
	}
	if byXbox.ID != igdbOnly.ID {
		t.Errorf("xbox lookup id = %d, want merged igdb row %d", byXbox.ID, igdbOnly.ID)
	}

	got, err := GetGameByIGDBID(ctx, db, igdbID)
	if err != nil {
		t.Fatalf("GetGameByIGDBID() error = %v", err)
	}
	if got.ID != igdbOnly.ID {
		t.Errorf("stored id = %d, want %d", got.ID, igdbOnly.ID)
	}
	if got.XboxTitleID == nil || *got.XboxTitleID != xboxTitleID {
		t.Errorf("stored xbox_title_id = %v, want %d", got.XboxTitleID, xboxTitleID)
	}
}

func TestGetGameByXboxTitleIDNotFound(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	_, err := GetGameByXboxTitleID(context.Background(), db, 999999999)
	if err != pgx.ErrNoRows {
		t.Fatalf("GetGameByXboxTitleID() error = %v, want ErrNoRows", err)
	}
}

func TestRemoveXboxGamesFromLibrary(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	user, err := CreateUser(ctx, db, fmt.Sprintf("xbox_clear_%d", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	cat, err := GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}

	xboxGame, err := FindOrCreateGameByXboxTitleID(ctx, db, 1144039928, "Halo Infinite", "", cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGameByXboxTitleID() error = %v", err)
	}
	manualGame, err := FindOrCreateGame(ctx, db, 99002, "Manual Game", "", 2020, []string{"PC"}, cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGame() error = %v", err)
	}

	if err := AddToLibrary(ctx, db, user.ID, xboxGame.ID, "Xbox", nil); err != nil {
		t.Fatalf("AddToLibrary(xbox) error = %v", err)
	}
	if err := AddToLibrary(ctx, db, user.ID, manualGame.ID, "PC", nil); err != nil {
		t.Fatalf("AddToLibrary(manual) error = %v", err)
	}

	removed, err := RemoveXboxGamesFromLibrary(ctx, db, user.ID)
	if err != nil {
		t.Fatalf("RemoveXboxGamesFromLibrary() error = %v", err)
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

func TestListImportedXboxTitleIDsIncludesSoftDeleted(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	user, err := CreateUser(ctx, db, fmt.Sprintf("xbox_imported_%d", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	cat, err := GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}

	game, err := FindOrCreateGameByXboxTitleID(ctx, db, 1144039928, "Halo Infinite", "", cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGameByXboxTitleID() error = %v", err)
	}
	if err := AddToLibrary(ctx, db, user.ID, game.ID, "Xbox", nil); err != nil {
		t.Fatalf("AddToLibrary() error = %v", err)
	}
	if err := RemoveFromLibrary(ctx, db, user.ID, game.ID); err != nil {
		t.Fatalf("RemoveFromLibrary() error = %v", err)
	}

	ids, err := ListImportedXboxTitleIDs(ctx, db, user.ID, "Xbox")
	if err != nil {
		t.Fatalf("ListImportedXboxTitleIDs() error = %v", err)
	}
	if _, ok := ids[1144039928]; !ok {
		t.Fatal("ListImportedXboxTitleIDs() missing soft-deleted Xbox title")
	}
}
