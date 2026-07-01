package models

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// LinkedPlatformPlaytime is total playtime for one linked platform.
type LinkedPlatformPlaytime struct {
	Platform string
	Minutes  int
}

// PlatformForProvider maps linked_accounts.provider to user_games.platform.
func PlatformForProvider(provider string) string {
	switch provider {
	case "steam":
		return "Steam"
	case "xbox":
		return "Xbox"
	default:
		return ""
	}
}

// ListLinkedPlatformPlaytime returns total playtime per linked account provider.
// Platforms without library entries report 0 minutes.
func ListLinkedPlatformPlaytime(ctx context.Context, db *pgxpool.Pool, userID int64) ([]LinkedPlatformPlaytime, error) {
	const query = `
		SELECT
			CASE la.provider
				WHEN 'steam' THEN 'Steam'
				WHEN 'xbox' THEN 'Xbox'
			END AS platform,
			COALESCE(SUM(ug.playtime_minutes), 0)::bigint AS total_minutes
		FROM linked_accounts la
		LEFT JOIN user_games ug
			ON ug.user_id = la.user_id
			AND ug.deleted_at IS NULL
			AND ug.platform = CASE la.provider
				WHEN 'steam' THEN 'Steam'
				WHEN 'xbox' THEN 'Xbox'
			END
		WHERE la.user_id = $1
		GROUP BY la.provider
		ORDER BY platform ASC
	`
	rows, err := db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var totals []LinkedPlatformPlaytime
	for rows.Next() {
		var item LinkedPlatformPlaytime
		if err := rows.Scan(&item.Platform, &item.Minutes); err != nil {
			return nil, err
		}
		totals = append(totals, item)
	}
	return totals, rows.Err()
}
