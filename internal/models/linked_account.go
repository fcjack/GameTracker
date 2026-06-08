package models

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type LinkedAccount struct {
	ID              int64
	UserID          int64
	Provider        string
	ExternalID      string
	AccessTokenEnc  string
	RefreshTokenEnc string
	TokenExpiresAt  *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func UpsertLinkedAccount(ctx context.Context, db *pgxpool.Pool, userID int64, provider, externalID, accessTokenEnc, refreshTokenEnc string, expiresAt *time.Time) (*LinkedAccount, error) {
	const query = `
		INSERT INTO linked_accounts (user_id, provider, external_id, access_token_enc, refresh_token_enc, token_expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		ON CONFLICT (user_id, provider) DO UPDATE SET
			external_id = $3,
			access_token_enc = $4,
			refresh_token_enc = $5,
			token_expires_at = $6,
			updated_at = NOW()
		RETURNING id, user_id, provider, external_id, access_token_enc, refresh_token_enc, token_expires_at, created_at, updated_at
	`

	var la LinkedAccount
	err := db.QueryRow(ctx, query, userID, provider, externalID, accessTokenEnc, refreshTokenEnc, expiresAt).Scan(
		&la.ID, &la.UserID, &la.Provider, &la.ExternalID, &la.AccessTokenEnc, &la.RefreshTokenEnc, &la.TokenExpiresAt, &la.CreatedAt, &la.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &la, nil
}

func GetLinkedAccount(ctx context.Context, db *pgxpool.Pool, userID int64, provider string) (*LinkedAccount, error) {
	const query = `
		SELECT id, user_id, provider, external_id, access_token_enc, refresh_token_enc, token_expires_at, created_at, updated_at
		FROM linked_accounts
		WHERE user_id = $1 AND provider = $2
	`

	var la LinkedAccount
	err := db.QueryRow(ctx, query, userID, provider).Scan(
		&la.ID, &la.UserID, &la.Provider, &la.ExternalID, &la.AccessTokenEnc, &la.RefreshTokenEnc, &la.TokenExpiresAt, &la.CreatedAt, &la.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &la, nil
}

func DeleteLinkedAccount(ctx context.Context, db *pgxpool.Pool, userID int64, provider string) error {
	const query = `DELETE FROM linked_accounts WHERE user_id = $1 AND provider = $2`
	_, err := db.Exec(ctx, query, userID, provider)
	return err
}

func ListLinkedAccounts(ctx context.Context, db *pgxpool.Pool, userID int64) ([]*LinkedAccount, error) {
	const query = `
		SELECT id, user_id, provider, external_id, access_token_enc, refresh_token_enc, token_expires_at, created_at, updated_at
		FROM linked_accounts
		WHERE user_id = $1
		ORDER BY created_at DESC
	`

	rows, err := db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []*LinkedAccount
	for rows.Next() {
		var la LinkedAccount
		err := rows.Scan(
			&la.ID, &la.UserID, &la.Provider, &la.ExternalID, &la.AccessTokenEnc, &la.RefreshTokenEnc, &la.TokenExpiresAt, &la.CreatedAt, &la.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, &la)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return accounts, nil
}
