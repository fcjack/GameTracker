package models

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	Locale       string
}

func CreateUser(ctx context.Context, db *pgxpool.Pool, username, password string) (*User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	var user User
	err = db.QueryRow(ctx,
		`INSERT INTO users (username, password_hash) VALUES ($1, $2)
		 RETURNING id, username, password_hash, locale`,
		username, string(hash),
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Locale)
	return &user, err
}

func GetUserByUsername(ctx context.Context, db *pgxpool.Pool, username string) (*User, error) {
	var user User
	err := db.QueryRow(ctx,
		`SELECT id, username, password_hash, locale FROM users WHERE username = $1`,
		username,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Locale)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func GetUserByID(ctx context.Context, db *pgxpool.Pool, userID int64) (*User, error) {
	var user User
	err := db.QueryRow(ctx,
		`SELECT id, username, password_hash, locale FROM users WHERE id = $1`,
		userID,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Locale)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func UpdatePassword(ctx context.Context, db *pgxpool.Pool, userID int64, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx,
		`UPDATE users SET password_hash = $2 WHERE id = $1`,
		userID, string(hash),
	)
	return err
}

func (u *User) CheckPassword(password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) == nil
}

func UpdateAvatar(ctx context.Context, db *pgxpool.Pool, userID int64, data []byte, mime string) error {
	const query = `UPDATE users SET avatar_data = $2, avatar_mime = $3 WHERE id = $1`
	_, err := db.Exec(ctx, query, userID, data, mime)
	return err
}

func GetUserLocale(ctx context.Context, db *pgxpool.Pool, userID int64) (string, error) {
	var locale string
	err := db.QueryRow(ctx, `SELECT locale FROM users WHERE id = $1`, userID).Scan(&locale)
	return locale, err
}

func UpdateUserLocale(ctx context.Context, db *pgxpool.Pool, userID int64, locale string) error {
	_, err := db.Exec(ctx, `UPDATE users SET locale = $2 WHERE id = $1`, userID, locale)
	return err
}

func GetAvatarByUserID(ctx context.Context, db *pgxpool.Pool, userID int64) ([]byte, string, error) {
	const query = `SELECT avatar_data, avatar_mime FROM users WHERE id = $1`
	var data []byte
	var mime string
	err := db.QueryRow(ctx, query, userID).Scan(&data, &mime)
	if err != nil {
		return nil, "", err
	}
	return data, mime, nil
}
