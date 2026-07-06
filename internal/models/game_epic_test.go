package models

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestFindOrCreateGameByEpicCatalogItemID(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	cat, err := GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}

	const catalogItemID = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
	coverURL := "https://cdn.example.com/epic/cover.jpg"
	game, err := FindOrCreateGameByEpicCatalogItemID(ctx, db, catalogItemID, "Epic Only", coverURL, cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGameByEpicCatalogItemID() error = %v", err)
	}
	if game.IGDBId != nil {
		t.Errorf("igdb_id = %v, want nil", game.IGDBId)
	}
	if game.EpicCatalogItemID == nil || *game.EpicCatalogItemID != catalogItemID {
		t.Errorf("epic_catalog_item_id = %v, want %q", game.EpicCatalogItemID, catalogItemID)
	}
	if game.EpicNamespace != "egs" {
		t.Errorf("epic_namespace = %q, want egs", game.EpicNamespace)
	}
	if game.CoverURL != coverURL {
		t.Errorf("cover_url = %q, want %q", game.CoverURL, coverURL)
	}

	again, err := FindOrCreateGameByEpicCatalogItemID(ctx, db, catalogItemID, "Epic Only Renamed", coverURL, cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGameByEpicCatalogItemID() second call error = %v", err)
	}
	if again.ID != game.ID {
		t.Errorf("id = %d, want same row %d", again.ID, game.ID)
	}
	if again.Name != "Epic Only Renamed" {
		t.Errorf("name = %q, want updated name", again.Name)
	}
}

func TestLinkIGDBToEpicGame(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	cat, err := GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}

	const catalogItemID = "b2c3d4e5-f6a7-8901-bcde-f12345678901"
	epicOnly, err := FindOrCreateGameByEpicCatalogItemID(ctx, db, catalogItemID, "Linkable Epic Game", "https://cdn.example.com/epic.jpg", cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGameByEpicCatalogItemID() error = %v", err)
	}

	linked, err := LinkIGDBToEpicGame(
		ctx, db, catalogItemID, 98767,
		"Linkable Epic Game IGDB", "https://cdn.example.com/igdb.jpg",
		2021, []string{"PC (Microsoft Windows)"}, cat.ID,
	)
	if err != nil {
		t.Fatalf("LinkIGDBToEpicGame() error = %v", err)
	}
	if linked.ID != epicOnly.ID {
		t.Errorf("id = %d, want %d", linked.ID, epicOnly.ID)
	}
	if linked.IGDBId == nil || *linked.IGDBId != 98767 {
		t.Errorf("igdb_id = %v, want 98767", linked.IGDBId)
	}

	got, err := GetGameByEpicCatalogItemID(ctx, db, catalogItemID)
	if err != nil {
		t.Fatalf("GetGameByEpicCatalogItemID() error = %v", err)
	}
	if got.IGDBId == nil || *got.IGDBId != 98767 {
		t.Errorf("stored igdb_id = %v, want 98767", got.IGDBId)
	}
}

func TestResolveGameForEpicImportMergesDuplicateRows(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	cat, err := GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}

	suffix := time.Now().UnixNano() % 1000000
	igdbID := int64(9988000000 + suffix)
	catalogItemID := fmt.Sprintf("c3d4e5f6-a7b8-9012-cdef-%012d", suffix)

	igdbOnly, err := FindOrCreateGameWithEpicCatalogItemID(
		ctx, db, igdbID, nil,
		"Merged Epic Game IGDB", "https://cdn.example.com/igdb.jpg",
		2020, []string{"PC (Microsoft Windows)"}, cat.ID,
	)
	if err != nil {
		t.Fatalf("FindOrCreateGameWithEpicCatalogItemID() igdb row error = %v", err)
	}

	epicOnly, err := FindOrCreateGameByEpicCatalogItemID(
		ctx, db, catalogItemID, "Merged Epic Game", "https://cdn.example.com/epic.jpg", cat.ID,
	)
	if err != nil {
		t.Fatalf("FindOrCreateGameByEpicCatalogItemID() epic row error = %v", err)
	}
	if epicOnly.ID == igdbOnly.ID {
		t.Fatal("expected separate rows before merge")
	}

	resolved, err := ResolveGameForEpicImport(
		ctx, db, catalogItemID, igdbID,
		"Merged Epic Game Final", "https://cdn.example.com/final.jpg",
		2020, []string{"PC (Microsoft Windows)", "Epic Games Store"}, cat.ID,
	)
	if err != nil {
		t.Fatalf("ResolveGameForEpicImport() error = %v", err)
	}
	if resolved.ID != igdbOnly.ID {
		t.Errorf("resolved id = %d, want canonical igdb row %d", resolved.ID, igdbOnly.ID)
	}
	if resolved.EpicCatalogItemID == nil || *resolved.EpicCatalogItemID != catalogItemID {
		t.Errorf("epic_catalog_item_id = %v, want %q", resolved.EpicCatalogItemID, catalogItemID)
	}
	if resolved.IGDBId == nil || *resolved.IGDBId != igdbID {
		t.Errorf("igdb_id = %v, want %d", resolved.IGDBId, igdbID)
	}

	byEpic, err := GetGameByEpicCatalogItemID(ctx, db, catalogItemID)
	if err != nil {
		t.Fatalf("GetGameByEpicCatalogItemID() after merge error = %v", err)
	}
	if byEpic.ID != igdbOnly.ID {
		t.Errorf("epic lookup id = %d, want merged igdb row %d", byEpic.ID, igdbOnly.ID)
	}

	got, err := GetGameByIGDBID(ctx, db, igdbID)
	if err != nil {
		t.Fatalf("GetGameByIGDBID() error = %v", err)
	}
	if got.ID != igdbOnly.ID {
		t.Errorf("stored id = %d, want %d", got.ID, igdbOnly.ID)
	}
	if got.EpicCatalogItemID == nil || *got.EpicCatalogItemID != catalogItemID {
		t.Errorf("stored epic_catalog_item_id = %v, want %q", got.EpicCatalogItemID, catalogItemID)
	}
}

func TestResolveGameForEpicImportBackfillsReleaseYear(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	cat, err := GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}

	const catalogItemID = "d4e5f6a7-b8c9-0123-def0-123456789012"
	igdbID := int64(556677881)

	epicOnly, err := FindOrCreateGameByEpicCatalogItemID(ctx, db, catalogItemID, "Yearless Epic Game", "https://cdn.example.com/epic.jpg", cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGameByEpicCatalogItemID() error = %v", err)
	}
	linked, err := LinkIGDBToEpicGame(
		ctx, db, catalogItemID, igdbID,
		"Yearless Epic Game", "https://cdn.example.com/epic.jpg",
		0, []string{"Epic"}, cat.ID,
	)
	if err != nil {
		t.Fatalf("LinkIGDBToEpicGame() error = %v", err)
	}
	if linked.ReleaseYear != 0 {
		t.Fatalf("release_year = %d, want 0 before backfill", linked.ReleaseYear)
	}

	resolved, err := ResolveGameForEpicImport(
		ctx, db, catalogItemID, igdbID,
		"Yearless Epic Game", "https://cdn.example.com/epic.jpg",
		2019, []string{"Epic Games Store"}, cat.ID,
	)
	if err != nil {
		t.Fatalf("ResolveGameForEpicImport() error = %v", err)
	}
	if resolved.ID != epicOnly.ID {
		t.Errorf("resolved id = %d, want %d", resolved.ID, epicOnly.ID)
	}
	if resolved.ReleaseYear != 2019 {
		t.Errorf("release_year = %d, want 2019", resolved.ReleaseYear)
	}
}

func TestGetGameByEpicCatalogItemIDNotFound(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	_, err := GetGameByEpicCatalogItemID(context.Background(), db, "00000000-0000-0000-0000-000000000000")
	if err != pgx.ErrNoRows {
		t.Fatalf("GetGameByEpicCatalogItemID() error = %v, want ErrNoRows", err)
	}
}

func TestRemoveEpicGamesFromLibrary(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	user, err := CreateUser(ctx, db, fmt.Sprintf("epic_clear_%d", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	cat, err := GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}

	epicGame, err := FindOrCreateGameByEpicCatalogItemID(ctx, db, "e5f6a7b8-c9d0-1234-ef01-234567890123", "Fortnite", "", cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGameByEpicCatalogItemID() error = %v", err)
	}
	manualGame, err := FindOrCreateGame(ctx, db, 99003, "Manual Game", "", 2020, []string{"PC"}, cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGame() error = %v", err)
	}

	if err := AddToLibrary(ctx, db, user.ID, epicGame.ID, "Epic", nil); err != nil {
		t.Fatalf("AddToLibrary(epic) error = %v", err)
	}
	if err := AddToLibrary(ctx, db, user.ID, manualGame.ID, "PC", nil); err != nil {
		t.Fatalf("AddToLibrary(manual) error = %v", err)
	}

	removed, err := RemoveEpicGamesFromLibrary(ctx, db, user.ID)
	if err != nil {
		t.Fatalf("RemoveEpicGamesFromLibrary() error = %v", err)
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

func TestListImportedEpicCatalogItemIDsIncludesSoftDeleted(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	user, err := CreateUser(ctx, db, fmt.Sprintf("epic_imported_%d", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	cat, err := GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}

	const catalogItemID = "f6a7b8c9-d0e1-2345-f012-345678901234"
	game, err := FindOrCreateGameByEpicCatalogItemID(ctx, db, catalogItemID, "Control", "", cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGameByEpicCatalogItemID() error = %v", err)
	}
	if err := AddToLibrary(ctx, db, user.ID, game.ID, "Epic", nil); err != nil {
		t.Fatalf("AddToLibrary() error = %v", err)
	}
	if err := RemoveFromLibrary(ctx, db, user.ID, game.ID); err != nil {
		t.Fatalf("RemoveFromLibrary() error = %v", err)
	}

	ids, err := ListImportedEpicCatalogItemIDs(ctx, db, user.ID, "Epic")
	if err != nil {
		t.Fatalf("ListImportedEpicCatalogItemIDs() error = %v", err)
	}
	if _, ok := ids[catalogItemID]; !ok {
		t.Fatal("ListImportedEpicCatalogItemIDs() missing soft-deleted Epic catalog item")
	}
}

func TestUpsertEpicLinkedAccount(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	user, err := CreateUser(ctx, db, fmt.Sprintf("epic_link_%d", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	expires := time.Now().Add(time.Hour)
	created, err := UpsertLinkedAccount(ctx, db, user.ID, "epic", "epic-account-123", "EpicGamer", "enc-access", "enc-refresh", &expires)
	if err != nil {
		t.Fatalf("UpsertLinkedAccount() error = %v", err)
	}
	if created.Provider != "epic" {
		t.Errorf("provider = %q, want epic", created.Provider)
	}
}
