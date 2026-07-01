package models

import (
	"context"
	"testing"
	"time"
)

func TestLinkGameFromIGDBXboxTitle(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	cat, err := GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}

	suffix := time.Now().UnixNano() % 1000000
	xboxTitleID := int(882000000 + suffix)
	igdbID := int64(883000000 + suffix)

	xboxOnly, err := FindOrCreateGameByXboxTitleID(
		ctx, db, xboxTitleID, "Manual Link Game", "https://cdn.example.com/xbox.jpg", cat.ID,
	)
	if err != nil {
		t.Fatalf("FindOrCreateGameByXboxTitleID() error = %v", err)
	}

	linked, err := LinkGameFromIGDB(
		ctx, db, xboxOnly.ID, igdbID,
		"Manual Link Game IGDB", "https://cdn.example.com/igdb.jpg",
		2019, []string{"Xbox One"}, cat.ID,
	)
	if err != nil {
		t.Fatalf("LinkGameFromIGDB() error = %v", err)
	}
	if linked.ID != xboxOnly.ID {
		t.Errorf("id = %d, want %d", linked.ID, xboxOnly.ID)
	}
	if linked.IGDBId == nil || *linked.IGDBId != igdbID {
		t.Errorf("igdb_id = %v, want %d", linked.IGDBId, igdbID)
	}
	if linked.ReleaseYear != 2019 {
		t.Errorf("release_year = %d, want 2019", linked.ReleaseYear)
	}
}
