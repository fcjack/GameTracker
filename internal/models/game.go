package models

import (
	"context"
	"fmt"
	"sort"
	"time"

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
	CategoryID  int64
	Name        string
	CoverURL    string
	Platforms   []string
	ReleaseYear int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type UserGame struct {
	UserID    int64
	GameID    int64
	Status    string
	Tags      []string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type UserGameWithGame struct {
	GameID       int64
	IGDBId       *int64
	Name         string
	CoverURL     string
	Platforms    []string
	ReleaseYear  int
	CategoryName string
	Status       string
	Tags         []string
	AddedAt      time.Time
}

type GamesByPlatform struct {
	Platform string
	Games    []*UserGameWithGame
}

func GroupUserGamesByPlatform(games []*UserGameWithGame) []GamesByPlatform {
	platformMap := make(map[string][]*UserGameWithGame)
	for _, g := range games {
		if len(g.Platforms) == 0 {
			platformMap["Unknown"] = append(platformMap["Unknown"], g)
			continue
		}
		for _, p := range g.Platforms {
			platformMap[p] = append(platformMap[p], g)
		}
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

func FindOrCreateGame(
	ctx context.Context,
	db *pgxpool.Pool,
	igdbID int64,
	name, coverURL string,
	releaseYear int,
	platforms []string,
	categoryID int64,
) (*Game, error) {
	const query = `
		INSERT INTO games (igdb_id, category_id, name, cover_url, platforms, release_year, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		ON CONFLICT (igdb_id) DO UPDATE SET
			name         = EXCLUDED.name,
			cover_url    = EXCLUDED.cover_url,
			platforms    = EXCLUDED.platforms,
			release_year = EXCLUDED.release_year,
			updated_at   = NOW()
		RETURNING id, igdb_id, category_id, name, cover_url, platforms, release_year, created_at, updated_at
	`
	var g Game
	err := db.QueryRow(ctx, query,
		igdbID, categoryID, name, coverURL, platforms, releaseYear,
	).Scan(
		&g.ID, &g.IGDBId, &g.CategoryID, &g.Name, &g.CoverURL, &g.Platforms, &g.ReleaseYear,
		&g.CreatedAt, &g.UpdatedAt,
	)
	return &g, err
}

func AddToLibrary(ctx context.Context, db *pgxpool.Pool, userID, gameID int64) error {
	const query = `
		INSERT INTO user_games (user_id, game_id, status, tags, created_at, updated_at)
		VALUES ($1, $2, 'owned', '{}', NOW(), NOW())
		ON CONFLICT (user_id, game_id) DO NOTHING
	`
	_, err := db.Exec(ctx, query, userID, gameID)
	return err
}

func ListUserGames(ctx context.Context, db *pgxpool.Pool, userID int64) ([]*UserGameWithGame, error) {
	const query = `
		SELECT
			g.id, g.igdb_id, g.name, g.cover_url, g.platforms,
			g.release_year, c.name AS category_name,
			ug.status, ug.tags, ug.created_at
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
			&ug.GameID, &ug.IGDBId, &ug.Name, &ug.CoverURL, &ug.Platforms,
			&ug.ReleaseYear, &ug.CategoryName,
			&ug.Status, &ug.Tags, &ug.AddedAt,
		)
		if err != nil {
			return nil, err
		}
		list = append(list, &ug)
	}
	return list, rows.Err()
}

func RemoveFromLibrary(ctx context.Context, db *pgxpool.Pool, userID, gameID int64) error {
	const query = `DELETE FROM user_games WHERE user_id = $1 AND game_id = $2`
	_, err := db.Exec(ctx, query, userID, gameID)
	return err
}

func UpdateGameStatus(ctx context.Context, db *pgxpool.Pool, userID, gameID int64, status string) error {
	const query = `
		UPDATE user_games SET status = $3, updated_at = NOW()
		WHERE user_id = $1 AND game_id = $2
	`
	ct, err := db.Exec(ctx, query, userID, gameID, status)
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
