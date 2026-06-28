package models

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jacksoncoelho/game-tracker/internal/database"
)

func TestLinkedAccountCRUD(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := t.Context()
	user, err := CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	expires := time.Now().Add(time.Hour)

	created, err := UpsertLinkedAccount(ctx, db, user.ID, "steam", "76561198012345678", "SteamGamer", "enc-access", "enc-refresh", &expires)
	if err != nil {
		t.Fatalf("UpsertLinkedAccount() create error = %v", err)
	}
	if created.Provider != "steam" {
		t.Errorf("provider = %q, want steam", created.Provider)
	}
	if created.ExternalID != "76561198012345678" {
		t.Errorf("external_id = %q, want steam id", created.ExternalID)
	}
	if created.DisplayName != "SteamGamer" {
		t.Errorf("display_name = %q, want SteamGamer", created.DisplayName)
	}

	got, err := GetLinkedAccount(ctx, db, user.ID, "steam")
	if err != nil {
		t.Fatalf("GetLinkedAccount() error = %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("GetLinkedAccount() id = %d, want %d", got.ID, created.ID)
	}

	updated, err := UpsertLinkedAccount(ctx, db, user.ID, "steam", "76561198099999999", "RenamedGamer", "", "", nil)
	if err != nil {
		t.Fatalf("UpsertLinkedAccount() update error = %v", err)
	}
	if updated.ID != created.ID {
		t.Errorf("upsert should keep same row id, got %d want %d", updated.ID, created.ID)
	}
	if updated.ExternalID != "76561198099999999" {
		t.Errorf("updated external_id = %q", updated.ExternalID)
	}
	if updated.DisplayName != "RenamedGamer" {
		t.Errorf("updated display_name = %q, want RenamedGamer", updated.DisplayName)
	}

	accounts, err := ListLinkedAccounts(ctx, db, user.ID)
	if err != nil {
		t.Fatalf("ListLinkedAccounts() error = %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("ListLinkedAccounts() count = %d, want 1", len(accounts))
	}
	if accounts[0].Provider != "steam" {
		t.Errorf("listed provider = %q, want steam", accounts[0].Provider)
	}

	if err := DeleteLinkedAccount(ctx, db, user.ID, "steam"); err != nil {
		t.Fatalf("DeleteLinkedAccount() error = %v", err)
	}

	accounts, err = ListLinkedAccounts(ctx, db, user.ID)
	if err != nil {
		t.Fatalf("ListLinkedAccounts() after delete error = %v", err)
	}
	if len(accounts) != 0 {
		t.Errorf("ListLinkedAccounts() after delete count = %d, want 0", len(accounts))
	}
}

func TestLinkedAccountProviderConstraint(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := t.Context()
	user, err := CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	_, err = UpsertLinkedAccount(ctx, db, user.ID, "invalid", "ext-id", "name", "", "", nil)
	if err == nil {
		t.Fatal("expected error for invalid provider, got nil")
	}
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

// withBoundedPool caps the per-test connection pool so parallel tests do not
// exhaust the database's connection limit. Each test only needs a couple of
// connections, and concurrency is already bounded by -parallel.
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

// chdirToModuleRoot moves to the module root exactly once per package test
// binary. Using sync.Once keeps it safe to call from parallel tests, since
// os.Chdir mutates process-global state.
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

func uniqueUsername(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("testuser_%d", time.Now().UnixNano())
}
