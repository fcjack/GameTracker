package models

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type GameCoverSources struct {
	SteamAppID *int
	IGDBId     *int64
	Name       string
	CoverURL   string
}

func SaveGameCover(ctx context.Context, db *pgxpool.Pool, gameID int64, data []byte, mime, sourceURL string) error {
	const query = `
		UPDATE games
		SET cover_data = $2,
		    cover_mime = $3,
		    cover_url = CASE WHEN $4 <> '' THEN $4 ELSE cover_url END,
		    updated_at = NOW()
		WHERE id = $1
	`
	_, err := db.Exec(ctx, query, gameID, data, mime, sourceURL)
	return err
}

func GetGameCoverData(ctx context.Context, db *pgxpool.Pool, gameID int64) ([]byte, string, error) {
	const query = `
		SELECT cover_data, cover_mime
		FROM games
		WHERE id = $1
	`
	var data []byte
	var mime *string
	err := db.QueryRow(ctx, query, gameID).Scan(&data, &mime)
	if err != nil {
		return nil, "", err
	}
	if data == nil {
		data = []byte{}
	}
	mimeStr := ""
	if mime != nil {
		mimeStr = *mime
	}
	return data, mimeStr, nil
}

func GetGameCoverSources(ctx context.Context, db *pgxpool.Pool, gameID int64) (*GameCoverSources, error) {
	const query = `
		SELECT steam_app_id, igdb_id, name, cover_url
		FROM games
		WHERE id = $1
	`
	var src GameCoverSources
	err := db.QueryRow(ctx, query, gameID).Scan(&src.SteamAppID, &src.IGDBId, &src.Name, &src.CoverURL)
	if err != nil {
		return nil, err
	}
	return &src, nil
}
