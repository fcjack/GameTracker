package xbox

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jacksoncoelho/game-tracker/internal/crypto"
	"github.com/jacksoncoelho/game-tracker/internal/models"
)

const tokenRefreshSkew = 5 * time.Minute

func tokenNeedsRefresh(expiresAt *time.Time, now time.Time) bool {
	if expiresAt == nil {
		return true
	}
	return !now.Before(expiresAt.Add(-tokenRefreshSkew))
}

// EnsureFreshTokens loads the user's linked Xbox account, refreshes OAuth tokens
// when expired or near expiry, persists updated encrypted tokens, and returns
// the usable token pair.
func EnsureFreshTokens(
	ctx context.Context,
	client *Client,
	enc *crypto.Encrypter,
	db *pgxpool.Pool,
	userID int64,
) (*TokenPair, error) {
	account, err := models.GetLinkedAccount(ctx, db, userID, "xbox")
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("xbox: account not linked")
		}
		return nil, err
	}

	accessToken, err := enc.Decrypt(account.AccessTokenEnc)
	if err != nil {
		return nil, fmt.Errorf("xbox: decrypt access token: %w", err)
	}

	refreshToken := ""
	if account.RefreshTokenEnc != "" {
		refreshToken, err = enc.Decrypt(account.RefreshTokenEnc)
		if err != nil {
			return nil, fmt.Errorf("xbox: decrypt refresh token: %w", err)
		}
	}

	if !tokenNeedsRefresh(account.TokenExpiresAt, time.Now()) {
		expiresAt := time.Now().Add(time.Hour)
		if account.TokenExpiresAt != nil {
			expiresAt = *account.TokenExpiresAt
		}
		return &TokenPair{
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
			ExpiresAt:    expiresAt,
		}, nil
	}

	if refreshToken == "" {
		return nil, fmt.Errorf("xbox: refresh token missing")
	}

	tokens, err := client.RefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, err
	}

	accessEnc, err := enc.Encrypt(tokens.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("xbox: encrypt access token: %w", err)
	}

	refreshEnc := account.RefreshTokenEnc
	if tokens.RefreshToken != "" {
		refreshEnc, err = enc.Encrypt(tokens.RefreshToken)
		if err != nil {
			return nil, fmt.Errorf("xbox: encrypt refresh token: %w", err)
		}
	}

	expiresAt := tokens.ExpiresAt
	_, err = models.UpsertLinkedAccount(
		ctx,
		db,
		userID,
		"xbox",
		account.ExternalID,
		account.DisplayName,
		accessEnc,
		refreshEnc,
		&expiresAt,
	)
	if err != nil {
		return nil, err
	}

	return tokens, nil
}
