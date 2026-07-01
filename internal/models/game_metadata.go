package models

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jacksoncoelho/game-tracker/internal/igdb"
)

// GameIGDBMetadata stores cached IGDB detail fields for the game detail page.
type GameIGDBMetadata struct {
	Summary          string            `json:"summary,omitempty"`
	Storyline        string            `json:"storyline,omitempty"`
	Genres           []string          `json:"genres,omitempty"`
	Themes           []string          `json:"themes,omitempty"`
	Keywords         []string          `json:"keywords,omitempty"`
	Developers       []string          `json:"developers,omitempty"`
	Publishers       []string          `json:"publishers,omitempty"`
	Platforms        []string          `json:"platforms,omitempty"`
	ReleaseDate      int64             `json:"release_date,omitempty"`
	GameStatus       int               `json:"game_status,omitempty"`
	AggregatedRating *float64          `json:"aggregated_rating,omitempty"`
	TotalRating      *float64          `json:"total_rating,omitempty"`
	RatingCount      int               `json:"rating_count,omitempty"`
	TotalRatingCount int               `json:"total_rating_count,omitempty"`
	BackdropURL      string            `json:"backdrop_url,omitempty"`
	ExternalLinks    map[string]string `json:"external_links,omitempty"`
	FetchedAt        time.Time         `json:"fetched_at,omitempty"`
}

func GameIGDBMetadataFromDetails(d *igdb.GameDetails) *GameIGDBMetadata {
	if d == nil {
		return nil
	}
	meta := &GameIGDBMetadata{
		Summary:          d.Summary,
		Storyline:        d.Storyline,
		Genres:           d.GenreNames(),
		Themes:           d.ThemeNames(),
		Keywords:         d.KeywordNames(),
		Developers:       d.DeveloperNames(),
		Publishers:       d.PublisherNames(),
		Platforms:        d.PlatformNames(),
		ReleaseDate:      d.FirstReleaseDate,
		GameStatus:       d.GameStatus,
		RatingCount:      d.RatingCount,
		TotalRatingCount: d.TotalRatingCount,
		BackdropURL:      igdb.BackdropURL(d),
		ExternalLinks:    d.ExternalLinks(),
		FetchedAt:        time.Now().UTC(),
	}
	if d.AggregatedRating > 0 {
		v := d.AggregatedRating
		meta.AggregatedRating = &v
	}
	if d.TotalRating > 0 {
		v := d.TotalRating
		meta.TotalRating = &v
	}
	return meta
}

func GetGameIGDBMetadata(ctx context.Context, db *pgxpool.Pool, gameID int64) (*GameIGDBMetadata, error) {
	const query = `SELECT igdb_metadata FROM games WHERE id = $1`
	var raw []byte
	err := db.QueryRow(ctx, query, gameID).Scan(&raw)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var meta GameIGDBMetadata
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func SaveGameIGDBMetadata(ctx context.Context, db *pgxpool.Pool, gameID int64, meta *GameIGDBMetadata) error {
	if meta == nil {
		return nil
	}
	meta.FetchedAt = time.Now().UTC()
	raw, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	const query = `
		UPDATE games
		SET igdb_metadata = $2, updated_at = NOW()
		WHERE id = $1
	`
	_, err = db.Exec(ctx, query, gameID, raw)
	return err
}
