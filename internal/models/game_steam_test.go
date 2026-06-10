package models

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestFindOrCreateGameBySteamAppID(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	cat, err := GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}

	coverURL := "https://cdn.example.com/steam/123.jpg"
	game, err := FindOrCreateGameBySteamAppID(ctx, db, 123, "Steam Only", coverURL, cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGameBySteamAppID() error = %v", err)
	}
	if game.IGDBId != nil {
		t.Errorf("igdb_id = %v, want nil", game.IGDBId)
	}
	if game.SteamAppID == nil || *game.SteamAppID != 123 {
		t.Errorf("steam_app_id = %v, want 123", game.SteamAppID)
	}
	if game.CoverURL != coverURL {
		t.Errorf("cover_url = %q, want %q", game.CoverURL, coverURL)
	}

	again, err := FindOrCreateGameBySteamAppID(ctx, db, 123, "Steam Only Renamed", coverURL, cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGameBySteamAppID() second call error = %v", err)
	}
	if again.ID != game.ID {
		t.Errorf("id = %d, want same row %d", again.ID, game.ID)
	}
	if again.Name != "Steam Only Renamed" {
		t.Errorf("name = %q, want updated name", again.Name)
	}
}

func TestLinkIGDBToSteamGame(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	cat, err := GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}

	steamOnly, err := FindOrCreateGameBySteamAppID(ctx, db, 456, "Linkable Game", "https://cdn.example.com/456.jpg", cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGameBySteamAppID() error = %v", err)
	}

	linked, err := LinkIGDBToSteamGame(
		ctx, db, 456, 98765,
		"Linkable Game IGDB", "https://cdn.example.com/igdb.jpg",
		2020, []string{"PC"}, cat.ID,
	)
	if err != nil {
		t.Fatalf("LinkIGDBToSteamGame() error = %v", err)
	}
	if linked.ID != steamOnly.ID {
		t.Errorf("id = %d, want %d", linked.ID, steamOnly.ID)
	}
	if linked.IGDBId == nil || *linked.IGDBId != 98765 {
		t.Errorf("igdb_id = %v, want 98765", linked.IGDBId)
	}

	got, err := GetGameBySteamAppID(ctx, db, 456)
	if err != nil {
		t.Fatalf("GetGameBySteamAppID() error = %v", err)
	}
	if got.IGDBId == nil || *got.IGDBId != 98765 {
		t.Errorf("stored igdb_id = %v, want 98765", got.IGDBId)
	}
}

func TestResolveGameForSteamImportMergesDuplicateRows(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	cat, err := GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}

	suffix := time.Now().UnixNano() % 1000000
	igdbID := int64(9988000000 + suffix)
	steamAppID := int(9988001000 + int(suffix))

	igdbOnly, err := FindOrCreateGameWithSteamAppID(
		ctx, db, igdbID, nil,
		"Merged Game IGDB", "https://cdn.example.com/igdb.jpg",
		2019, []string{"PC"}, cat.ID,
	)
	if err != nil {
		t.Fatalf("FindOrCreateGameWithSteamAppID() igdb row error = %v", err)
	}

	steamOnly, err := FindOrCreateGameBySteamAppID(
		ctx, db, steamAppID, "Merged Game Steam", "https://cdn.example.com/steam.jpg", cat.ID,
	)
	if err != nil {
		t.Fatalf("FindOrCreateGameBySteamAppID() steam row error = %v", err)
	}
	if steamOnly.ID == igdbOnly.ID {
		t.Fatal("expected separate rows before merge")
	}

	resolved, err := ResolveGameForSteamImport(
		ctx, db, steamAppID, igdbID,
		"Merged Game Final", "https://cdn.example.com/final.jpg",
		2019, []string{"PC", "Steam"}, cat.ID,
	)
	if err != nil {
		t.Fatalf("ResolveGameForSteamImport() error = %v", err)
	}
	if resolved.ID != igdbOnly.ID {
		t.Errorf("resolved id = %d, want canonical igdb row %d", resolved.ID, igdbOnly.ID)
	}
	if resolved.SteamAppID == nil || *resolved.SteamAppID != steamAppID {
		t.Errorf("steam_app_id = %v, want %d", resolved.SteamAppID, steamAppID)
	}
	if resolved.IGDBId == nil || *resolved.IGDBId != igdbID {
		t.Errorf("igdb_id = %v, want %d", resolved.IGDBId, igdbID)
	}

	bySteam, err := GetGameBySteamAppID(ctx, db, steamAppID)
	if err != nil {
		t.Fatalf("GetGameBySteamAppID() after merge error = %v", err)
	}
	if bySteam.ID != igdbOnly.ID {
		t.Errorf("steam lookup id = %d, want merged igdb row %d", bySteam.ID, igdbOnly.ID)
	}

	got, err := GetGameByIGDBID(ctx, db, igdbID)
	if err != nil {
		t.Fatalf("GetGameByIGDBID() error = %v", err)
	}
	if got.ID != igdbOnly.ID {
		t.Errorf("stored id = %d, want %d", got.ID, igdbOnly.ID)
	}
	if got.SteamAppID == nil || *got.SteamAppID != steamAppID {
		t.Errorf("stored steam_app_id = %v, want %d", got.SteamAppID, steamAppID)
	}
}

func TestGetGameBySteamAppIDNotFound(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	_, err := GetGameBySteamAppID(context.Background(), db, 999999)
	if err != pgx.ErrNoRows {
		t.Fatalf("GetGameBySteamAppID() error = %v, want ErrNoRows", err)
	}
}
