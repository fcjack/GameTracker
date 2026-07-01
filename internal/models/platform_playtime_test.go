package models

import (
	"context"
	"testing"
)

func TestPlatformForProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		provider string
		want     string
	}{
		{"steam", "Steam"},
		{"xbox", "Xbox"},
		{"invalid", ""},
	}

	for _, tt := range tests {
		if got := PlatformForProvider(tt.provider); got != tt.want {
			t.Errorf("PlatformForProvider(%q) = %q, want %q", tt.provider, got, tt.want)
		}
	}
}

func TestListLinkedPlatformPlaytime(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	defer db.Close()

	user, err := CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	cat, err := GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}

	totals, err := ListLinkedPlatformPlaytime(ctx, db, user.ID)
	if err != nil {
		t.Fatalf("ListLinkedPlatformPlaytime() error = %v", err)
	}
	if len(totals) != 0 {
		t.Fatalf("ListLinkedPlatformPlaytime() with no links = %v, want empty", totals)
	}

	if _, err := UpsertLinkedAccount(ctx, db, user.ID, "steam", "76561198012345678", "SteamGamer", "", "", nil); err != nil {
		t.Fatalf("UpsertLinkedAccount(steam) error = %v", err)
	}
	if _, err := UpsertLinkedAccount(ctx, db, user.ID, "xbox", "2535465432123456", "XboxGamer", "", "", nil); err != nil {
		t.Fatalf("UpsertLinkedAccount(xbox) error = %v", err)
	}

	steamGame, err := FindOrCreateGameBySteamAppID(ctx, db, 570, "Dota 2", "", cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGameBySteamAppID() error = %v", err)
	}
	steamPlaytime := 150
	if err := AddToLibrary(ctx, db, user.ID, steamGame.ID, "Steam", &steamPlaytime); err != nil {
		t.Fatalf("AddToLibrary(steam) error = %v", err)
	}

	xboxGame, err := FindOrCreateGameByXboxTitleID(ctx, db, 1144039928, "Halo Infinite", "", cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGameByXboxTitleID() error = %v", err)
	}
	xboxPlaytime := 90
	if err := AddToLibrary(ctx, db, user.ID, xboxGame.ID, "Xbox", &xboxPlaytime); err != nil {
		t.Fatalf("AddToLibrary(xbox) error = %v", err)
	}

	totals, err = ListLinkedPlatformPlaytime(ctx, db, user.ID)
	if err != nil {
		t.Fatalf("ListLinkedPlatformPlaytime() error = %v", err)
	}
	if len(totals) != 2 {
		t.Fatalf("ListLinkedPlatformPlaytime() count = %d, want 2: %v", len(totals), totals)
	}

	byPlatform := make(map[string]int, len(totals))
	for _, item := range totals {
		byPlatform[item.Platform] = item.Minutes
	}
	if byPlatform["Steam"] != 150 {
		t.Errorf("Steam playtime = %d, want 150", byPlatform["Steam"])
	}
	if byPlatform["Xbox"] != 90 {
		t.Errorf("Xbox playtime = %d, want 90", byPlatform["Xbox"])
	}
}

func TestListLinkedPlatformPlaytime_zeroWhenNoGames(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	defer db.Close()

	user, err := CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if _, err := UpsertLinkedAccount(ctx, db, user.ID, "steam", "76561198012345678", "SteamGamer", "", "", nil); err != nil {
		t.Fatalf("UpsertLinkedAccount() error = %v", err)
	}

	totals, err := ListLinkedPlatformPlaytime(ctx, db, user.ID)
	if err != nil {
		t.Fatalf("ListLinkedPlatformPlaytime() error = %v", err)
	}
	if len(totals) != 1 {
		t.Fatalf("ListLinkedPlatformPlaytime() count = %d, want 1", len(totals))
	}
	if totals[0].Platform != "Steam" || totals[0].Minutes != 0 {
		t.Fatalf("totals[0] = %+v, want Steam with 0 minutes", totals[0])
	}
}
