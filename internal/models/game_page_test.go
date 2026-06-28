package models

import (
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func seedLibrary(t *testing.T, db *pgxpool.Pool, userID int64, igdbBase int64, names []string) []*Game {
	t.Helper()
	ctx := t.Context()

	cat, err := GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}

	games := make([]*Game, 0, len(names))
	for i, name := range names {
		game, err := FindOrCreateGame(ctx, db, igdbBase+int64(i), name, "", 2020, []string{"PC"}, cat.ID)
		if err != nil {
			t.Fatalf("FindOrCreateGame() error = %v", err)
		}
		if err := AddToLibrary(ctx, db, userID, game.ID, "PC", nil); err != nil {
			t.Fatalf("AddToLibrary() error = %v", err)
		}
		games = append(games, game)
	}
	return games
}

func TestListUserGamesPage_paginates(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := t.Context()
	user, err := CreateUser(ctx, db, fmt.Sprintf("page_user_%d", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	names := []string{"Game A", "Game B", "Game C", "Game D", "Game E"}
	seedLibrary(t, db, user.ID, 96000, names)

	page1, err := ListUserGamesPage(ctx, db, user.ID, 2, 0)
	if err != nil {
		t.Fatalf("ListUserGamesPage(page 1) error = %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page 1 returned %d games, want 2", len(page1))
	}

	page2, err := ListUserGamesPage(ctx, db, user.ID, 2, 2)
	if err != nil {
		t.Fatalf("ListUserGamesPage(page 2) error = %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("page 2 returned %d games, want 2", len(page2))
	}

	page3, err := ListUserGamesPage(ctx, db, user.ID, 2, 4)
	if err != nil {
		t.Fatalf("ListUserGamesPage(page 3) error = %v", err)
	}
	if len(page3) != 1 {
		t.Fatalf("page 3 returned %d games, want 1", len(page3))
	}

	beyond, err := ListUserGamesPage(ctx, db, user.ID, 2, 6)
	if err != nil {
		t.Fatalf("ListUserGamesPage(beyond) error = %v", err)
	}
	if len(beyond) != 0 {
		t.Fatalf("page beyond end returned %d games, want 0", len(beyond))
	}

	// Pages must not overlap.
	seen := make(map[int64]bool)
	for _, g := range append(append(page1, page2...), page3...) {
		if seen[g.GameID] {
			t.Fatalf("game %d returned on more than one page", g.GameID)
		}
		seen[g.GameID] = true
	}
	if len(seen) != len(names) {
		t.Fatalf("pages covered %d distinct games, want %d", len(seen), len(names))
	}
}

func TestListUserGamesPage_zeroLimitReturnsAll(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := t.Context()
	user, err := CreateUser(ctx, db, fmt.Sprintf("page_all_%d", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	seedLibrary(t, db, user.ID, 96100, []string{"One", "Two", "Three"})

	all, err := ListUserGamesPage(ctx, db, user.ID, 0, 0)
	if err != nil {
		t.Fatalf("ListUserGamesPage() error = %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("returned %d games, want 3", len(all))
	}
}

func TestListUserGamesByStatusesPage_paginates(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := t.Context()
	user, err := CreateUser(ctx, db, fmt.Sprintf("page_status_%d", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	games := seedLibrary(t, db, user.ID, 96200, []string{"P1", "P2", "P3", "Owned"})
	for _, g := range games[:3] {
		if err := UpdateGameStatus(ctx, db, user.ID, g.ID, "playing"); err != nil {
			t.Fatalf("UpdateGameStatus() error = %v", err)
		}
	}

	page1, err := ListUserGamesByStatusesPage(ctx, db, user.ID, []string{"playing"}, 2, 0)
	if err != nil {
		t.Fatalf("ListUserGamesByStatusesPage(page 1) error = %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page 1 returned %d games, want 2", len(page1))
	}

	page2, err := ListUserGamesByStatusesPage(ctx, db, user.ID, []string{"playing"}, 2, 2)
	if err != nil {
		t.Fatalf("ListUserGamesByStatusesPage(page 2) error = %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("page 2 returned %d games, want 1", len(page2))
	}

	for _, g := range append(page1, page2...) {
		if g.Status != "playing" {
			t.Fatalf("game %d has status %q, want playing", g.GameID, g.Status)
		}
	}
}

func TestLibraryIGDBIDs(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := t.Context()
	user, err := CreateUser(ctx, db, fmt.Sprintf("igdbids_user_%d", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	other, err := CreateUser(ctx, db, fmt.Sprintf("igdbids_other_%d", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	games := seedLibrary(t, db, user.ID, 96300, []string{"Kept", "Removed"})
	seedLibrary(t, db, other.ID, 96400, []string{"Other User Game"})

	if err := RemoveFromLibrary(ctx, db, user.ID, games[1].ID); err != nil {
		t.Fatalf("RemoveFromLibrary() error = %v", err)
	}

	ids := []int64{96300, 96301, 96400, 99999}
	inLibrary, err := LibraryIGDBIDs(ctx, db, user.ID, ids)
	if err != nil {
		t.Fatalf("LibraryIGDBIDs() error = %v", err)
	}

	if _, ok := inLibrary[96300]; !ok {
		t.Error("expected igdb id 96300 to be in library")
	}
	if _, ok := inLibrary[96301]; ok {
		t.Error("soft-deleted igdb id 96301 should not be in library")
	}
	if _, ok := inLibrary[96400]; ok {
		t.Error("other user's igdb id 96400 should not be in library")
	}
	if _, ok := inLibrary[99999]; ok {
		t.Error("unknown igdb id 99999 should not be in library")
	}
	if len(inLibrary) != 1 {
		t.Errorf("LibraryIGDBIDs() returned %d ids, want 1", len(inLibrary))
	}
}

func TestLibraryIGDBIDs_emptyInput(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := t.Context()
	user, err := CreateUser(ctx, db, fmt.Sprintf("igdbids_empty_%d", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	inLibrary, err := LibraryIGDBIDs(ctx, db, user.ID, nil)
	if err != nil {
		t.Fatalf("LibraryIGDBIDs() error = %v", err)
	}
	if len(inLibrary) != 0 {
		t.Errorf("LibraryIGDBIDs() returned %d ids, want 0", len(inLibrary))
	}
}
