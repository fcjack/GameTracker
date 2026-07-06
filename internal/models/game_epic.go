package models

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func GetGameByEpicCatalogItemID(ctx context.Context, db *pgxpool.Pool, catalogItemID string) (*Game, error) {
	const query = `
		SELECT id, igdb_id, steam_app_id, xbox_title_id, epic_catalog_item_id, epic_namespace, category_id, name, cover_url, platforms, release_year, created_at, updated_at
		FROM games
		WHERE epic_catalog_item_id = $1
	`
	var g Game
	err := db.QueryRow(ctx, query, catalogItemID).Scan(
		&g.ID, &g.IGDBId, &g.SteamAppID, &g.XboxTitleID, &g.EpicCatalogItemID, &g.EpicNamespace, &g.CategoryID, &g.Name, &g.CoverURL, &g.Platforms, &g.ReleaseYear,
		&g.CreatedAt, &g.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// ResolveGameForEpicImport returns the canonical games row for an Epic title matched to IGDB,
// merging duplicate epic-only and igdb-only rows when both exist.
func ResolveGameForEpicImport(
	ctx context.Context,
	db *pgxpool.Pool,
	catalogItemID string,
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

	epicGame, err := GetGameByEpicCatalogItemID(ctx, db, catalogItemID)
	if err != nil && err != pgx.ErrNoRows {
		return nil, err
	}

	if igdbGame != nil && epicGame != nil && igdbGame.ID != epicGame.ID {
		if err := MergeGameInto(ctx, db, epicGame.ID, igdbGame.ID); err != nil {
			return nil, err
		}
		return FindOrCreateGameWithEpicCatalogItemID(
			ctx, db, igdbID, &catalogItemID,
			name, coverURL, releaseYear, platforms, categoryID,
		)
	}

	if igdbGame != nil {
		return FindOrCreateGameWithEpicCatalogItemID(
			ctx, db, igdbID, &catalogItemID,
			name, coverURL, releaseYear, platforms, categoryID,
		)
	}

	if epicGame != nil {
		if epicGame.IGDBId != nil {
			if releaseYear > 0 && epicGame.ReleaseYear <= 0 {
				return LinkIGDBToEpicGame(
					ctx, db, catalogItemID, *epicGame.IGDBId,
					name, coverURL, releaseYear, platforms, categoryID,
				)
			}
			return epicGame, nil
		}
		return LinkIGDBToEpicGame(
			ctx, db, catalogItemID, igdbID,
			name, coverURL, releaseYear, platforms, categoryID,
		)
	}

	return FindOrCreateGameWithEpicCatalogItemID(
		ctx, db, igdbID, &catalogItemID,
		name, coverURL, releaseYear, platforms, categoryID,
	)
}

// ApplyEpicImportMetadata updates canonical game display fields from Epic data.
// Epic cover takes precedence; fallbackCover (e.g. IGDB) is used only when epicCover is empty.
func ApplyEpicImportMetadata(ctx context.Context, db *pgxpool.Pool, gameID int64, name, epicCover, fallbackCover string) error {
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
	_, err := db.Exec(ctx, query, gameID, name, epicCover, fallbackCover)
	return err
}

func FindOrCreateGameByEpicCatalogItemID(
	ctx context.Context,
	db *pgxpool.Pool,
	catalogItemID string,
	name, coverURL string,
	categoryID int64,
) (*Game, error) {
	const query = `
		INSERT INTO games (epic_catalog_item_id, epic_namespace, igdb_id, category_id, name, cover_url, platforms, release_year, created_at, updated_at)
		VALUES ($1, 'egs', NULL, $2, $3, $4, ARRAY['Epic'], 0, NOW(), NOW())
		ON CONFLICT (epic_catalog_item_id) DO UPDATE SET
			name      = EXCLUDED.name,
			cover_url = CASE WHEN EXCLUDED.cover_url <> '' THEN EXCLUDED.cover_url ELSE games.cover_url END,
			updated_at = NOW()
		RETURNING id, igdb_id, steam_app_id, xbox_title_id, epic_catalog_item_id, epic_namespace, category_id, name, cover_url, platforms, release_year, created_at, updated_at
	`
	var g Game
	err := db.QueryRow(ctx, query, catalogItemID, categoryID, name, coverURL).Scan(
		&g.ID, &g.IGDBId, &g.SteamAppID, &g.XboxTitleID, &g.EpicCatalogItemID, &g.EpicNamespace, &g.CategoryID, &g.Name, &g.CoverURL, &g.Platforms, &g.ReleaseYear,
		&g.CreatedAt, &g.UpdatedAt,
	)
	return &g, err
}

func FindOrCreateGameWithEpicCatalogItemID(
	ctx context.Context,
	db *pgxpool.Pool,
	igdbID int64,
	catalogItemID *string,
	name, coverURL string,
	releaseYear int,
	platforms []string,
	categoryID int64,
) (*Game, error) {
	const query = `
		INSERT INTO games (igdb_id, epic_catalog_item_id, epic_namespace, category_id, name, cover_url, platforms, release_year, created_at, updated_at)
		VALUES ($1, $2, 'egs', $3, $4, $5, $6, $7, NOW(), NOW())
		ON CONFLICT (igdb_id) DO UPDATE SET
			epic_catalog_item_id = COALESCE(EXCLUDED.epic_catalog_item_id, games.epic_catalog_item_id),
			name         = EXCLUDED.name,
			cover_url    = EXCLUDED.cover_url,
			platforms    = EXCLUDED.platforms,
			release_year = CASE WHEN EXCLUDED.release_year > 0 THEN EXCLUDED.release_year ELSE games.release_year END,
			updated_at   = NOW()
		RETURNING id, igdb_id, steam_app_id, xbox_title_id, epic_catalog_item_id, epic_namespace, category_id, name, cover_url, platforms, release_year, created_at, updated_at
	`
	var g Game
	err := db.QueryRow(ctx, query,
		igdbID, catalogItemID, categoryID, name, coverURL, platforms, releaseYear,
	).Scan(
		&g.ID, &g.IGDBId, &g.SteamAppID, &g.XboxTitleID, &g.EpicCatalogItemID, &g.EpicNamespace, &g.CategoryID, &g.Name, &g.CoverURL, &g.Platforms, &g.ReleaseYear,
		&g.CreatedAt, &g.UpdatedAt,
	)
	return &g, err
}

func LinkIGDBToEpicGame(
	ctx context.Context,
	db *pgxpool.Pool,
	catalogItemID string,
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
		WHERE epic_catalog_item_id = $1
		RETURNING id, igdb_id, steam_app_id, xbox_title_id, epic_catalog_item_id, epic_namespace, category_id, name, cover_url, platforms, release_year, created_at, updated_at
	`
	var g Game
	err := db.QueryRow(ctx, query,
		catalogItemID, igdbID, categoryID, name, coverURL, platforms, releaseYear,
	).Scan(
		&g.ID, &g.IGDBId, &g.SteamAppID, &g.XboxTitleID, &g.EpicCatalogItemID, &g.EpicNamespace, &g.CategoryID, &g.Name, &g.CoverURL, &g.Platforms, &g.ReleaseYear,
		&g.CreatedAt, &g.UpdatedAt,
	)
	return &g, err
}

// RemoveEpicGamesFromLibrary hard-deletes every Epic-platform entry for the user
// so a subsequent Epic sync can re-import from scratch.
func RemoveEpicGamesFromLibrary(ctx context.Context, db *pgxpool.Pool, userID int64) (int64, error) {
	const query = `DELETE FROM user_games WHERE user_id = $1 AND platform = 'Epic'`
	ct, err := db.Exec(ctx, query, userID)
	if err != nil {
		return 0, err
	}
	return ct.RowsAffected(), nil
}

// ListImportedEpicCatalogItemIDs returns Epic catalog item IDs already in the user's library on the given platform.
func ListImportedEpicCatalogItemIDs(ctx context.Context, db *pgxpool.Pool, userID int64, platform string) (map[string]struct{}, error) {
	const query = `
		SELECT g.epic_catalog_item_id
		FROM user_games ug
		JOIN games g ON g.id = ug.game_id
		WHERE ug.user_id = $1 AND ug.platform = $2 AND g.epic_catalog_item_id IS NOT NULL
	`
	rows, err := db.Query(ctx, query, userID, platform)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make(map[string]struct{})
	for rows.Next() {
		var catalogItemID string
		if err := rows.Scan(&catalogItemID); err != nil {
			return nil, err
		}
		ids[catalogItemID] = struct{}{}
	}
	return ids, rows.Err()
}
