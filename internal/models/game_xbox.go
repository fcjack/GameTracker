package models

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func GetGameByXboxTitleID(ctx context.Context, db *pgxpool.Pool, xboxTitleID int) (*Game, error) {
	const query = `
		SELECT id, igdb_id, steam_app_id, xbox_title_id, category_id, name, cover_url, platforms, release_year, created_at, updated_at
		FROM games
		WHERE xbox_title_id = $1
	`
	var g Game
	err := db.QueryRow(ctx, query, xboxTitleID).Scan(
		&g.ID, &g.IGDBId, &g.SteamAppID, &g.XboxTitleID, &g.CategoryID, &g.Name, &g.CoverURL, &g.Platforms, &g.ReleaseYear,
		&g.CreatedAt, &g.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// ResolveGameForXboxImport returns the canonical games row for an Xbox title matched to IGDB,
// merging duplicate xbox-only and igdb-only rows when both exist.
func ResolveGameForXboxImport(
	ctx context.Context,
	db *pgxpool.Pool,
	xboxTitleID int,
	igdbID int64,
	name, coverURL string,
	releaseYear int,
	platforms []string,
	categoryID int64,
) (*Game, error) {
	igdbGame, err := GetGameByIGDBID(ctx, db, igdbID)
	if err != nil && err != pgx.ErrNoRows {
		return nil, err
	}

	xboxGame, err := GetGameByXboxTitleID(ctx, db, xboxTitleID)
	if err != nil && err != pgx.ErrNoRows {
		return nil, err
	}

	if igdbGame != nil && xboxGame != nil && igdbGame.ID != xboxGame.ID {
		if err := MergeGameInto(ctx, db, xboxGame.ID, igdbGame.ID); err != nil {
			return nil, err
		}
		return FindOrCreateGameWithXboxTitleID(
			ctx, db, igdbID, &xboxTitleID,
			name, coverURL, releaseYear, platforms, categoryID,
		)
	}

	if igdbGame != nil {
		return FindOrCreateGameWithXboxTitleID(
			ctx, db, igdbID, &xboxTitleID,
			name, coverURL, releaseYear, platforms, categoryID,
		)
	}

	if xboxGame != nil {
		if xboxGame.IGDBId != nil {
			return xboxGame, nil
		}
		return LinkIGDBToXboxGame(
			ctx, db, xboxTitleID, igdbID,
			name, coverURL, releaseYear, platforms, categoryID,
		)
	}

	return FindOrCreateGameWithXboxTitleID(
		ctx, db, igdbID, &xboxTitleID,
		name, coverURL, releaseYear, platforms, categoryID,
	)
}

// ApplyXboxImportMetadata updates canonical game display fields from Xbox data.
// Xbox cover takes precedence; fallbackCover (e.g. IGDB) is used only when xboxCover is empty.
func ApplyXboxImportMetadata(ctx context.Context, db *pgxpool.Pool, gameID int64, name, xboxCover, fallbackCover string) error {
	const query = `
		UPDATE games
		SET name = $2,
		    cover_url = CASE
		        WHEN $3 <> '' THEN $3
		        WHEN $4 <> '' THEN $4
		        ELSE cover_url
		    END,
		    updated_at = NOW()
		WHERE id = $1
	`
	_, err := db.Exec(ctx, query, gameID, name, xboxCover, fallbackCover)
	return err
}

func FindOrCreateGameByXboxTitleID(
	ctx context.Context,
	db *pgxpool.Pool,
	xboxTitleID int,
	name, coverURL string,
	categoryID int64,
) (*Game, error) {
	const query = `
		INSERT INTO games (xbox_title_id, igdb_id, category_id, name, cover_url, platforms, release_year, created_at, updated_at)
		VALUES ($1, NULL, $2, $3, $4, ARRAY['Xbox'], 0, NOW(), NOW())
		ON CONFLICT (xbox_title_id) DO UPDATE SET
			name      = EXCLUDED.name,
			cover_url = CASE WHEN EXCLUDED.cover_url <> '' THEN EXCLUDED.cover_url ELSE games.cover_url END,
			updated_at = NOW()
		RETURNING id, igdb_id, steam_app_id, xbox_title_id, category_id, name, cover_url, platforms, release_year, created_at, updated_at
	`
	var g Game
	err := db.QueryRow(ctx, query, xboxTitleID, categoryID, name, coverURL).Scan(
		&g.ID, &g.IGDBId, &g.SteamAppID, &g.XboxTitleID, &g.CategoryID, &g.Name, &g.CoverURL, &g.Platforms, &g.ReleaseYear,
		&g.CreatedAt, &g.UpdatedAt,
	)
	return &g, err
}

func FindOrCreateGameWithXboxTitleID(
	ctx context.Context,
	db *pgxpool.Pool,
	igdbID int64,
	xboxTitleID *int,
	name, coverURL string,
	releaseYear int,
	platforms []string,
	categoryID int64,
) (*Game, error) {
	const query = `
		INSERT INTO games (igdb_id, xbox_title_id, category_id, name, cover_url, platforms, release_year, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
		ON CONFLICT (igdb_id) DO UPDATE SET
			xbox_title_id = COALESCE(EXCLUDED.xbox_title_id, games.xbox_title_id),
			name         = EXCLUDED.name,
			cover_url    = EXCLUDED.cover_url,
			platforms    = EXCLUDED.platforms,
			release_year = EXCLUDED.release_year,
			updated_at   = NOW()
		RETURNING id, igdb_id, steam_app_id, xbox_title_id, category_id, name, cover_url, platforms, release_year, created_at, updated_at
	`
	var g Game
	err := db.QueryRow(ctx, query,
		igdbID, xboxTitleID, categoryID, name, coverURL, platforms, releaseYear,
	).Scan(
		&g.ID, &g.IGDBId, &g.SteamAppID, &g.XboxTitleID, &g.CategoryID, &g.Name, &g.CoverURL, &g.Platforms, &g.ReleaseYear,
		&g.CreatedAt, &g.UpdatedAt,
	)
	return &g, err
}

func LinkIGDBToXboxGame(
	ctx context.Context,
	db *pgxpool.Pool,
	xboxTitleID int,
	igdbID int64,
	name, coverURL string,
	releaseYear int,
	platforms []string,
	categoryID int64,
) (*Game, error) {
	const query = `
		UPDATE games
		SET igdb_id = $2,
		    category_id = $3,
		    name = $4,
		    cover_url = CASE WHEN $5 <> '' THEN $5 ELSE cover_url END,
		    platforms = $6,
		    release_year = $7,
		    updated_at = NOW()
		WHERE xbox_title_id = $1
		RETURNING id, igdb_id, steam_app_id, xbox_title_id, category_id, name, cover_url, platforms, release_year, created_at, updated_at
	`
	var g Game
	err := db.QueryRow(ctx, query,
		xboxTitleID, igdbID, categoryID, name, coverURL, platforms, releaseYear,
	).Scan(
		&g.ID, &g.IGDBId, &g.SteamAppID, &g.XboxTitleID, &g.CategoryID, &g.Name, &g.CoverURL, &g.Platforms, &g.ReleaseYear,
		&g.CreatedAt, &g.UpdatedAt,
	)
	return &g, err
}

// RemoveXboxGamesFromLibrary hard-deletes every Xbox-platform entry for the user
// so a subsequent Xbox sync can re-import from scratch.
func RemoveXboxGamesFromLibrary(ctx context.Context, db *pgxpool.Pool, userID int64) (int64, error) {
	const query = `DELETE FROM user_games WHERE user_id = $1 AND platform = 'Xbox'`
	ct, err := db.Exec(ctx, query, userID)
	if err != nil {
		return 0, err
	}
	return ct.RowsAffected(), nil
}

// ListImportedXboxTitleIDs returns Xbox title IDs already in the user's library on the given platform.
func ListImportedXboxTitleIDs(ctx context.Context, db *pgxpool.Pool, userID int64, platform string) (map[int]struct{}, error) {
	const query = `
		SELECT g.xbox_title_id
		FROM user_games ug
		JOIN games g ON g.id = ug.game_id
		WHERE ug.user_id = $1 AND ug.platform = $2 AND g.xbox_title_id IS NOT NULL
	`
	rows, err := db.Query(ctx, query, userID, platform)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make(map[int]struct{})
	for rows.Next() {
		var titleID int
		if err := rows.Scan(&titleID); err != nil {
			return nil, err
		}
		ids[titleID] = struct{}{}
	}
	return ids, rows.Err()
}
