package xbox

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jacksoncoelho/game-tracker/internal/crypto"
	"github.com/jacksoncoelho/game-tracker/internal/database"
	"github.com/jacksoncoelho/game-tracker/internal/models"
)

func TestEnsureFreshTokensReturnsExistingAccessToken(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	enc := testEncrypter(t)

	user, err := models.CreateUser(ctx, db, fmt.Sprintf("xbox_tokens_%d", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	expiresAt := time.Now().Add(2 * time.Hour)
	accessEnc, err := enc.Encrypt("valid-access-token")
	if err != nil {
		t.Fatalf("Encrypt(access) error = %v", err)
	}
	refreshEnc, err := enc.Encrypt("valid-refresh-token")
	if err != nil {
		t.Fatalf("Encrypt(refresh) error = %v", err)
	}

	if _, err := models.UpsertLinkedAccount(
		ctx, db, user.ID, "xbox", "2535465432123456", "TestGamer",
		accessEnc, refreshEnc, &expiresAt,
	); err != nil {
		t.Fatalf("UpsertLinkedAccount() error = %v", err)
	}

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("token server should not be called for valid access token")
	}))
	defer tokenServer.Close()

	client := NewClientWithHTTP("client-id", "secret", tokenServer.Client())
	client.SetEndpoints(tokenServer.URL, "", "")

	tokens, err := EnsureFreshTokens(ctx, client, enc, db, user.ID)
	if err != nil {
		t.Fatalf("EnsureFreshTokens() error = %v", err)
	}
	if tokens.AccessToken != "valid-access-token" {
		t.Errorf("AccessToken = %q, want valid-access-token", tokens.AccessToken)
	}
	if tokens.RefreshToken != "valid-refresh-token" {
		t.Errorf("RefreshToken = %q, want valid-refresh-token", tokens.RefreshToken)
	}
}

func TestEnsureFreshTokensRefreshesExpiredToken(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	enc := testEncrypter(t)

	user, err := models.CreateUser(ctx, db, fmt.Sprintf("xbox_refresh_%d", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	expiresAt := time.Now().Add(-time.Hour)
	accessEnc, err := enc.Encrypt("expired-access-token")
	if err != nil {
		t.Fatalf("Encrypt(access) error = %v", err)
	}
	refreshEnc, err := enc.Encrypt("stored-refresh-token")
	if err != nil {
		t.Fatalf("Encrypt(refresh) error = %v", err)
	}

	if _, err := models.UpsertLinkedAccount(
		ctx, db, user.ID, "xbox", "2535465432123456", "TestGamer",
		accessEnc, refreshEnc, &expiresAt,
	); err != nil {
		t.Fatalf("UpsertLinkedAccount() error = %v", err)
	}

	const newAccess = "refreshed-access-token"
	const newRefresh = "refreshed-refresh-token"

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if got := r.Form.Get("refresh_token"); got != "stored-refresh-token" {
			t.Errorf("refresh_token = %q, want stored-refresh-token", got)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  newAccess,
			"refresh_token": newRefresh,
			"expires_in":    3600,
		})
	}))
	defer tokenServer.Close()

	client := NewClientWithHTTP("client-id", "secret", tokenServer.Client())
	client.SetEndpoints(tokenServer.URL, "", "")

	tokens, err := EnsureFreshTokens(ctx, client, enc, db, user.ID)
	if err != nil {
		t.Fatalf("EnsureFreshTokens() error = %v", err)
	}
	if tokens.AccessToken != newAccess {
		t.Errorf("AccessToken = %q, want %q", tokens.AccessToken, newAccess)
	}

	account, err := models.GetLinkedAccount(ctx, db, user.ID, "xbox")
	if err != nil {
		t.Fatalf("GetLinkedAccount() error = %v", err)
	}

	gotAccess, err := enc.Decrypt(account.AccessTokenEnc)
	if err != nil {
		t.Fatalf("Decrypt(access) error = %v", err)
	}
	if gotAccess != newAccess {
		t.Errorf("stored access token = %q, want %q", gotAccess, newAccess)
	}

	gotRefresh, err := enc.Decrypt(account.RefreshTokenEnc)
	if err != nil {
		t.Fatalf("Decrypt(refresh) error = %v", err)
	}
	if gotRefresh != newRefresh {
		t.Errorf("stored refresh token = %q, want %q", gotRefresh, newRefresh)
	}
	if account.TokenExpiresAt == nil || !account.TokenExpiresAt.After(time.Now()) {
		t.Errorf("TokenExpiresAt = %v, want future expiry", account.TokenExpiresAt)
	}
}

func testEncrypter(t *testing.T) *crypto.Encrypter {
	t.Helper()
	enc, err := crypto.NewEncrypter("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewEncrypter() error = %v", err)
	}
	return enc
}

func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	chdirToModuleRoot(t)

	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		dbURL = "postgres://gametracker:gametracker@localhost:5432/gametracker?sslmode=disable"
	}

	db, err := database.Connect(withBoundedPool(dbURL))
	if err != nil {
		t.Skipf("database not available: %v", err)
	}
	if err := database.RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations() error = %v", err)
	}
	return db
}

func withBoundedPool(dbURL string) string {
	if strings.Contains(dbURL, "pool_max_conns") {
		return dbURL
	}
	sep := "?"
	if strings.Contains(dbURL, "?") {
		sep = "&"
	}
	return dbURL + sep + "pool_max_conns=4"
}

var moduleRootOnce sync.Once

func chdirToModuleRoot(t *testing.T) {
	t.Helper()
	moduleRootOnce.Do(func() {
		dir, err := os.Getwd()
		if err != nil {
			panic(err)
		}
		for {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				if err := os.Chdir(dir); err != nil {
					panic(err)
				}
				return
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				panic("could not find module root")
			}
			dir = parent
		}
	})
}
