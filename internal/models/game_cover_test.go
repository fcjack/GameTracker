package models

import (
	"testing"
)

func TestSaveAndGetGameCover(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	ctx := t.Context()
	cat, err := GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}

	game, err := FindOrCreateGame(ctx, db, 88001, "Cover Test Game", "", 2024, []string{"PC"}, cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGame() error = %v", err)
	}

	data := []byte{0xFF, 0xD8, 0xFF, 0x00}
	sourceURL := "https://images.igdb.com/igdb/image/upload/t_cover_big/cotest.jpg"
	if err := SaveGameCover(ctx, db, game.ID, data, "image/jpeg", sourceURL); err != nil {
		t.Fatalf("SaveGameCover() error = %v", err)
	}

	gotData, gotMIME, err := GetGameCoverData(ctx, db, game.ID)
	if err != nil {
		t.Fatalf("GetGameCoverData() error = %v", err)
	}
	if string(gotData) != string(data) {
		t.Errorf("cover_data = %v, want %v", gotData, data)
	}
	if gotMIME != "image/jpeg" {
		t.Errorf("cover_mime = %q, want image/jpeg", gotMIME)
	}

	src, err := GetGameCoverSources(ctx, db, game.ID)
	if err != nil {
		t.Fatalf("GetGameCoverSources() error = %v", err)
	}
	if src.CoverURL != sourceURL {
		t.Errorf("cover_url = %q, want %q", src.CoverURL, sourceURL)
	}
	if src.Name != "Cover Test Game" {
		t.Errorf("name = %q, want Cover Test Game", src.Name)
	}
}
