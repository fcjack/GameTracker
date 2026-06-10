package models

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Category struct {
	ID        int64
	Name      string
	IGDBValue *int
}

type Game struct {
	ID          int64
	IGDBId      *int64
	SteamAppID  *int
	CategoryID  int64
	Name        string
	CoverURL    string
	Platforms   []string
	ReleaseYear int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type UserGame struct {
	UserID      int64
	GameID      int64
	Status      string
	Tags        []string
	CompletedAt *time.Time
	DroppedAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type UserGameWithGame struct {
	GameID       int64
	IGDBId       *int64
	Name         string
	CoverURL     string
	Platform     string
	ReleaseYear  int
	CategoryName string
	Status       string
	Tags         []string
	CompletedAt  *time.Time
	DroppedAt    *time.Time
	AddedAt      time.Time
}

type GamesByPlatform struct {
	Platform string
	Games    []*UserGameWithGame
}

type GamesByYear struct {
	Label string
	Games []*UserGameWithGame
}

func GroupUserGamesByPlatform(games []*UserGameWithGame) []GamesByPlatform {
	platformMap := make(map[string][]*UserGameWithGame)
	for _, g := range games {
		platform := g.Platform
		if platform == "" {
			platform = "Unknown"
		}
		platformMap[platform] = append(platformMap[platform], g)
	}

	platforms := make([]string, 0, len(platformMap))
	for p := range platformMap {
		platforms = append(platforms, p)
	}
	sort.Strings(platforms)

	groups := make([]GamesByPlatform, len(platforms))
	for i, p := range platforms {
		groups[i] = GamesByPlatform{Platform: p, Games: platformMap[p]}
	}
	return groups
}

func GroupUserGamesByCompletionYear(games []*UserGameWithGame) []GamesByYear {
	var active []*UserGameWithGame
	byYear := make(map[int][]*UserGameWithGame)

	for _, g := range games {
		if g.Status != "completed" || g.CompletedAt == nil {
			active = append(active, g)
			continue
		}
		year := g.CompletedAt.Year()
		byYear[year] = append(byYear[year], g)
	}

	years := make([]int, 0, len(byYear))
	for y := range byYear {
		years = append(years, y)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(years)))

	groups := make([]GamesByYear, 0, len(years)+1)
	if len(active) > 0 {
		groups = append(groups, GamesByYear{Label: "Active", Games: active})
	}
	for _, y := range years {
		groups = append(groups, GamesByYear{
			Label: fmt.Sprintf("%d", y),
			Games: byYear[y],
		})
	}
	return groups
}

func FindOrCreateGame(
	ctx context.Context,
	db *pgxpool.Pool,
	igdbID int64,
	name, coverURL string,
	releaseYear int,
	platforms []string,
	categoryID int64,
) (*Game, error) {
	return FindOrCreateGameWithSteamAppID(ctx, db, igdbID, nil, name, coverURL, releaseYear, platforms, categoryID)
}

func FindOrCreateGameWithSteamAppID(
	ctx context.Context,
	db *pgxpool.Pool,
	igdbID int64,
	steamAppID *int,
	name, coverURL string,
	releaseYear int,
	platforms []string,
	categoryID int64,
) (*Game, error) {
	const query = `
		INSERT INTO games (igdb_id, steam_app_id, category_id, name, cover_url, platforms, release_year, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
		ON CONFLICT (igdb_id) DO UPDATE SET
			steam_app_id = COALESCE(EXCLUDED.steam_app_id, games.steam_app_id),
			name         = EXCLUDED.name,
			cover_url    = EXCLUDED.cover_url,
			platforms    = EXCLUDED.platforms,
			release_year = EXCLUDED.release_year,
			updated_at   = NOW()
		RETURNING id, igdb_id, steam_app_id, category_id, name, cover_url, platforms, release_year, created_at, updated_at
	`
	var g Game
	err := db.QueryRow(ctx, query,
		igdbID, steamAppID, categoryID, name, coverURL, platforms, releaseYear,
	).Scan(
		&g.ID, &g.IGDBId, &g.SteamAppID, &g.CategoryID, &g.Name, &g.CoverURL, &g.Platforms, &g.ReleaseYear,
		&g.CreatedAt, &g.UpdatedAt,
	)
	return &g, err
}

func GetGameBySteamAppID(ctx context.Context, db *pgxpool.Pool, steamAppID int) (*Game, error) {
	const query = `
		SELECT id, igdb_id, steam_app_id, category_id, name, cover_url, platforms, release_year, created_at, updated_at
		FROM games
		WHERE steam_app_id = $1
	`
	var g Game
	err := db.QueryRow(ctx, query, steamAppID).Scan(
		&g.ID, &g.IGDBId, &g.SteamAppID, &g.CategoryID, &g.Name, &g.CoverURL, &g.Platforms, &g.ReleaseYear,
		&g.CreatedAt, &g.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

func GetGameByIGDBID(ctx context.Context, db *pgxpool.Pool, igdbID int64) (*Game, error) {
	const query = `
		SELECT id, igdb_id, steam_app_id, category_id, name, cover_url, platforms, release_year, created_at, updated_at
		FROM games
		WHERE igdb_id = $1
	`
	var g Game
	err := db.QueryRow(ctx, query, igdbID).Scan(
		&g.ID, &g.IGDBId, &g.SteamAppID, &g.CategoryID, &g.Name, &g.CoverURL, &g.Platforms, &g.ReleaseYear,
		&g.CreatedAt, &g.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// MergeGameInto moves library entries from fromID to toID and deletes the duplicate game row.
func MergeGameInto(ctx context.Context, db *pgxpool.Pool, fromID, toID int64) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	const dedupe = `
		DELETE FROM user_games ug_from
		WHERE ug_from.game_id = $1
		  AND EXISTS (
			SELECT 1 FROM user_games ug_to
			WHERE ug_to.game_id = $2 AND ug_to.user_id = ug_from.user_id
		  )
	`
	if _, err := tx.Exec(ctx, dedupe, fromID, toID); err != nil {
		return err
	}

	const move = `UPDATE user_games SET game_id = $2 WHERE game_id = $1`
	if _, err := tx.Exec(ctx, move, fromID, toID); err != nil {
		return err
	}

	const deleteGame = `DELETE FROM games WHERE id = $1`
	if _, err := tx.Exec(ctx, deleteGame, fromID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ResolveGameForSteamImport returns the canonical games row for a Steam title matched to IGDB,
// merging duplicate steam-only and igdb-only rows when both exist.
func ResolveGameForSteamImport(
	ctx context.Context,
	db *pgxpool.Pool,
	steamAppID int,
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

	steamGame, err := GetGameBySteamAppID(ctx, db, steamAppID)
	if err != nil && err != pgx.ErrNoRows {
		return nil, err
	}

	if igdbGame != nil && steamGame != nil && igdbGame.ID != steamGame.ID {
		if err := MergeGameInto(ctx, db, steamGame.ID, igdbGame.ID); err != nil {
			return nil, err
		}
		return FindOrCreateGameWithSteamAppID(
			ctx, db, igdbID, &steamAppID,
			name, coverURL, releaseYear, platforms, categoryID,
		)
	}

	if igdbGame != nil {
		return FindOrCreateGameWithSteamAppID(
			ctx, db, igdbID, &steamAppID,
			name, coverURL, releaseYear, platforms, categoryID,
		)
	}

	if steamGame != nil {
		if steamGame.IGDBId != nil {
			return steamGame, nil
		}
		return LinkIGDBToSteamGame(
			ctx, db, steamAppID, igdbID,
			name, coverURL, releaseYear, platforms, categoryID,
		)
	}

	return FindOrCreateGameWithSteamAppID(
		ctx, db, igdbID, &steamAppID,
		name, coverURL, releaseYear, platforms, categoryID,
	)
}

func FindOrCreateGameBySteamAppID(
	ctx context.Context,
	db *pgxpool.Pool,
	steamAppID int,
	name, coverURL string,
	categoryID int64,
) (*Game, error) {
	const query = `
		INSERT INTO games (steam_app_id, igdb_id, category_id, name, cover_url, platforms, release_year, created_at, updated_at)
		VALUES ($1, NULL, $2, $3, $4, ARRAY['Steam'], 0, NOW(), NOW())
		ON CONFLICT (steam_app_id) DO UPDATE SET
			name      = EXCLUDED.name,
			cover_url = CASE WHEN EXCLUDED.cover_url <> '' THEN EXCLUDED.cover_url ELSE games.cover_url END,
			updated_at = NOW()
		RETURNING id, igdb_id, steam_app_id, category_id, name, cover_url, platforms, release_year, created_at, updated_at
	`
	var g Game
	err := db.QueryRow(ctx, query, steamAppID, categoryID, name, coverURL).Scan(
		&g.ID, &g.IGDBId, &g.SteamAppID, &g.CategoryID, &g.Name, &g.CoverURL, &g.Platforms, &g.ReleaseYear,
		&g.CreatedAt, &g.UpdatedAt,
	)
	return &g, err
}

func LinkIGDBToSteamGame(
	ctx context.Context,
	db *pgxpool.Pool,
	steamAppID int,
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
		WHERE steam_app_id = $1
		RETURNING id, igdb_id, steam_app_id, category_id, name, cover_url, platforms, release_year, created_at, updated_at
	`
	var g Game
	err := db.QueryRow(ctx, query,
		steamAppID, igdbID, categoryID, name, coverURL, platforms, releaseYear,
	).Scan(
		&g.ID, &g.IGDBId, &g.SteamAppID, &g.CategoryID, &g.Name, &g.CoverURL, &g.Platforms, &g.ReleaseYear,
		&g.CreatedAt, &g.UpdatedAt,
	)
	return &g, err
}

func AddToLibrary(ctx context.Context, db *pgxpool.Pool, userID, gameID int64, platform string) error {
	const query = `
		INSERT INTO user_games (user_id, game_id, platform, status, tags, created_at, updated_at)
		VALUES ($1, $2, $3, 'owned', '{}', NOW(), NOW())
		ON CONFLICT (user_id, game_id) DO NOTHING
	`
	_, err := db.Exec(ctx, query, userID, gameID, platform)
	return err
}

func ListUserGames(ctx context.Context, db *pgxpool.Pool, userID int64) ([]*UserGameWithGame, error) {
	const query = `
		SELECT
			g.id, g.igdb_id, g.name, g.cover_url, ug.platform,
			g.release_year, c.name AS category_name,
			ug.status, ug.tags, ug.completed_at, ug.dropped_at, ug.created_at
		FROM user_games ug
		JOIN games g     ON g.id = ug.game_id
		JOIN categories c ON c.id = g.category_id
		WHERE ug.user_id = $1
		ORDER BY ug.created_at DESC
	`
	rows, err := db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*UserGameWithGame
	for rows.Next() {
		var ug UserGameWithGame
		err := rows.Scan(
			&ug.GameID, &ug.IGDBId, &ug.Name, &ug.CoverURL, &ug.Platform,
			&ug.ReleaseYear, &ug.CategoryName,
			&ug.Status, &ug.Tags, &ug.CompletedAt, &ug.DroppedAt, &ug.AddedAt,
		)
		if err != nil {
			return nil, err
		}
		list = append(list, &ug)
	}
	return list, rows.Err()
}

func ListUserGamesByStatuses(ctx context.Context, db *pgxpool.Pool, userID int64, statuses []string) ([]*UserGameWithGame, error) {
	const query = `
		SELECT
			g.id, g.igdb_id, g.name, g.cover_url, ug.platform,
			g.release_year, c.name AS category_name,
			ug.status, ug.tags, ug.completed_at, ug.dropped_at, ug.created_at
		FROM user_games ug
		JOIN games g     ON g.id = ug.game_id
		JOIN categories c ON c.id = g.category_id
		WHERE ug.user_id = $1 AND ug.status = ANY($2)
		ORDER BY ug.updated_at DESC
	`
	rows, err := db.Query(ctx, query, userID, statuses)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*UserGameWithGame
	for rows.Next() {
		var ug UserGameWithGame
		err := rows.Scan(
			&ug.GameID, &ug.IGDBId, &ug.Name, &ug.CoverURL, &ug.Platform,
			&ug.ReleaseYear, &ug.CategoryName,
			&ug.Status, &ug.Tags, &ug.CompletedAt, &ug.DroppedAt, &ug.AddedAt,
		)
		if err != nil {
			return nil, err
		}
		list = append(list, &ug)
	}
	return list, rows.Err()
}

func GetUserGame(ctx context.Context, db *pgxpool.Pool, userID, gameID int64) (*UserGameWithGame, error) {
	const query = `
		SELECT
			g.id, g.igdb_id, g.name, g.cover_url, ug.platform,
			g.release_year, c.name AS category_name,
			ug.status, ug.tags, ug.completed_at, ug.dropped_at, ug.created_at
		FROM user_games ug
		JOIN games g     ON g.id = ug.game_id
		JOIN categories c ON c.id = g.category_id
		WHERE ug.user_id = $1 AND ug.game_id = $2
	`
	var ug UserGameWithGame
	err := db.QueryRow(ctx, query, userID, gameID).Scan(
		&ug.GameID, &ug.IGDBId, &ug.Name, &ug.CoverURL, &ug.Platform,
		&ug.ReleaseYear, &ug.CategoryName,
		&ug.Status, &ug.Tags, &ug.CompletedAt, &ug.DroppedAt, &ug.AddedAt,
	)
	if err != nil {
		return nil, err
	}
	return &ug, nil
}

func RemoveFromLibrary(ctx context.Context, db *pgxpool.Pool, userID, gameID int64) error {
	const query = `DELETE FROM user_games WHERE user_id = $1 AND game_id = $2`
	_, err := db.Exec(ctx, query, userID, gameID)
	return err
}

func UpdateGameStatus(ctx context.Context, db *pgxpool.Pool, userID, gameID int64, status string) error {
	var query string
	switch status {
	case "completed":
		query = `
			UPDATE user_games
			SET status = $3, completed_at = COALESCE(completed_at, NOW()), dropped_at = NULL, updated_at = NOW()
			WHERE user_id = $1 AND game_id = $2
		`
	case "dropped":
		query = `
			UPDATE user_games
			SET status = $3, dropped_at = NOW(), completed_at = NULL, updated_at = NOW()
			WHERE user_id = $1 AND game_id = $2
		`
	default:
		query = `
			UPDATE user_games
			SET status = $3, completed_at = NULL, dropped_at = NULL, updated_at = NOW()
			WHERE user_id = $1 AND game_id = $2
		`
	}
	ct, err := db.Exec(ctx, query, userID, gameID, status)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("game not in library")
	}
	return nil
}

func MarkGameCompleted(ctx context.Context, db *pgxpool.Pool, userID, gameID int64, completedAt time.Time) error {
	const query = `
		UPDATE user_games
		SET status = 'completed', completed_at = $3, dropped_at = NULL, updated_at = NOW()
		WHERE user_id = $1 AND game_id = $2
	`
	ct, err := db.Exec(ctx, query, userID, gameID, completedAt)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("game not in library")
	}
	return nil
}

func GetCategoryByIGDBValue(ctx context.Context, db *pgxpool.Pool, igdbValue int) (*Category, error) {
	const query = `SELECT id, name, igdb_value FROM categories WHERE igdb_value = $1`
	var cat Category
	err := db.QueryRow(ctx, query, igdbValue).Scan(&cat.ID, &cat.Name, &cat.IGDBValue)
	if err != nil {
		return nil, err
	}
	return &cat, nil
}

func IsInLibrary(ctx context.Context, db *pgxpool.Pool, userID, gameID int64) (bool, error) {
	const query = `SELECT EXISTS(SELECT 1 FROM user_games WHERE user_id = $1 AND game_id = $2)`
	var exists bool
	err := db.QueryRow(ctx, query, userID, gameID).Scan(&exists)
	return exists, err
}

// ListImportedSteamAppIDs returns Steam app IDs already in the user's library on the given platform.
func ListImportedSteamAppIDs(ctx context.Context, db *pgxpool.Pool, userID int64, platform string) (map[int]struct{}, error) {
	const query = `
		SELECT g.steam_app_id
		FROM user_games ug
		JOIN games g ON g.id = ug.game_id
		WHERE ug.user_id = $1 AND ug.platform = $2 AND g.steam_app_id IS NOT NULL
	`
	rows, err := db.Query(ctx, query, userID, platform)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make(map[int]struct{})
	for rows.Next() {
		var appID int
		if err := rows.Scan(&appID); err != nil {
			return nil, err
		}
		ids[appID] = struct{}{}
	}
	return ids, rows.Err()
}

func GetGameStatistics(ctx context.Context, db *pgxpool.Pool, userID int64) (map[string]int, error) {
	const query = `
		SELECT status, COUNT(*) as count
		FROM user_games
		WHERE user_id = $1
		GROUP BY status
	`
	rows, err := db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := map[string]int{
		"owned":     0,
		"playing":   0,
		"completed": 0,
		"dropped":   0,
	}

	for rows.Next() {
		var status string
		var count int
		err := rows.Scan(&status, &count)
		if err != nil {
			return nil, err
		}
		stats[status] = count
	}
	return stats, rows.Err()
}

func GetCompletedCountByYear(ctx context.Context, db *pgxpool.Pool, userID int64, year int) (int, error) {
	const query = `
		SELECT COUNT(*)
		FROM user_games
		WHERE user_id = $1
		  AND status = 'completed'
		  AND completed_at IS NOT NULL
		  AND EXTRACT(YEAR FROM completed_at) = $2
	`
	var count int
	err := db.QueryRow(ctx, query, userID, year).Scan(&count)
	return count, err
}

func ListCompletionYears(ctx context.Context, db *pgxpool.Pool, userID int64) ([]int, error) {
	const query = `
		SELECT DISTINCT EXTRACT(YEAR FROM completed_at)::int AS year
		FROM user_games
		WHERE user_id = $1
		  AND status = 'completed'
		  AND completed_at IS NOT NULL
		ORDER BY year DESC
	`
	rows, err := db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var years []int
	for rows.Next() {
		var year int
		if err := rows.Scan(&year); err != nil {
			return nil, err
		}
		years = append(years, year)
	}
	return years, rows.Err()
}
