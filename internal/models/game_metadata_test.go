package models

import (
	"context"
	"testing"
	"time"

	"github.com/jacksoncoelho/game-tracker/internal/igdb"
)

func TestSaveAndGetGameIGDBMetadata(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	cat, err := GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}

	suffix := time.Now().UnixNano() % 1000000
	igdbID := int64(884000000 + suffix)
	game, err := FindOrCreateGame(ctx, db, igdbID, "Metadata Test", "", 2021, []string{"PC"}, cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGame() error = %v", err)
	}

	rating := 88.0
	meta := GameIGDBMetadataFromDetails(&igdb.GameDetails{
		Summary:          "A test summary.",
		AggregatedRating: rating,
		Genres:           []igdb.NamedEntity{{Name: "Action"}},
		FirstReleaseDate: time.Date(2021, 3, 1, 0, 0, 0, 0, time.UTC).Unix(),
	})
	if err := SaveGameIGDBMetadata(ctx, db, game.ID, meta); err != nil {
		t.Fatalf("SaveGameIGDBMetadata() error = %v", err)
	}

	got, err := GetGameIGDBMetadata(ctx, db, game.ID)
	if err != nil {
		t.Fatalf("GetGameIGDBMetadata() error = %v", err)
	}
	if got == nil || got.Summary != "A test summary." {
		t.Fatalf("metadata = %+v, want summary", got)
	}
	if got.AggregatedRating == nil || *got.AggregatedRating != 88 {
		t.Errorf("aggregated_rating = %v, want 88", got.AggregatedRating)
	}
	if len(got.Genres) != 1 || got.Genres[0] != "Action" {
		t.Errorf("genres = %v", got.Genres)
	}
}
