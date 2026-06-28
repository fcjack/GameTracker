package importjob

import (
	"context"
	"sync"
	"testing"

	"github.com/jacksoncoelho/game-tracker/internal/models"
)

type fakeImporter struct {
	mu       sync.Mutex
	steamIDs map[int64]string
	xboxIDs  map[int64]struct{}
}

func (f *fakeImporter) StartSteamImport(_ context.Context, userID int64, steamID string) (*models.ImportJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.steamIDs == nil {
		f.steamIDs = make(map[int64]string)
	}
	f.steamIDs[userID] = steamID
	return &models.ImportJob{UserID: userID, Provider: "steam"}, nil
}

func (f *fakeImporter) StartXboxImport(_ context.Context, userID int64) (*models.ImportJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.xboxIDs == nil {
		f.xboxIDs = make(map[int64]struct{})
	}
	f.xboxIDs[userID] = struct{}{}
	return &models.ImportJob{UserID: userID, Provider: "xbox"}, nil
}

func TestSchedulerSyncSteamTriggersAllLinkedAccounts(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()

	user1, err := models.CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	user2, err := models.CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	if _, err := models.UpsertLinkedAccount(ctx, db, user1.ID, "steam", "76561190000000001", "One", "", "", nil); err != nil {
		t.Fatalf("UpsertLinkedAccount(user1) error = %v", err)
	}
	if _, err := models.UpsertLinkedAccount(ctx, db, user2.ID, "steam", "76561190000000002", "Two", "", "", nil); err != nil {
		t.Fatalf("UpsertLinkedAccount(user2) error = %v", err)
	}

	importer := &fakeImporter{}
	s := NewScheduler(db, importer, 0)
	s.SyncSteam(ctx)

	if importer.steamIDs[user1.ID] != "76561190000000001" {
		t.Errorf("user1 import steamID = %q, want 76561190000000001", importer.steamIDs[user1.ID])
	}
	if importer.steamIDs[user2.ID] != "76561190000000002" {
		t.Errorf("user2 import steamID = %q, want 76561190000000002", importer.steamIDs[user2.ID])
	}
}

func TestSchedulerSyncXboxTriggersAllLinkedAccounts(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()

	user1, err := models.CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	user2, err := models.CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	if _, err := models.UpsertLinkedAccount(ctx, db, user1.ID, "xbox", "2535465432123456", "One", "", "", nil); err != nil {
		t.Fatalf("UpsertLinkedAccount(user1) error = %v", err)
	}
	if _, err := models.UpsertLinkedAccount(ctx, db, user2.ID, "xbox", "2535465432123457", "Two", "", "", nil); err != nil {
		t.Fatalf("UpsertLinkedAccount(user2) error = %v", err)
	}

	importer := &fakeImporter{}
	s := NewScheduler(db, importer, 0)
	s.SyncXbox(ctx)

	if _, ok := importer.xboxIDs[user1.ID]; !ok {
		t.Errorf("user1 Xbox import not triggered")
	}
	if _, ok := importer.xboxIDs[user2.ID]; !ok {
		t.Errorf("user2 Xbox import not triggered")
	}
}

func TestSchedulerSyncAllTriggersBothProviders(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()

	user, err := models.CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	if _, err := models.UpsertLinkedAccount(ctx, db, user.ID, "steam", "76561190000000001", "Steam", "", "", nil); err != nil {
		t.Fatalf("UpsertLinkedAccount(steam) error = %v", err)
	}
	if _, err := models.UpsertLinkedAccount(ctx, db, user.ID, "xbox", "2535465432123456", "Xbox", "", "", nil); err != nil {
		t.Fatalf("UpsertLinkedAccount(xbox) error = %v", err)
	}

	importer := &fakeImporter{}
	s := NewScheduler(db, importer, 0)
	s.SyncAll(ctx)

	if importer.steamIDs[user.ID] != "76561190000000001" {
		t.Errorf("Steam import steamID = %q, want 76561190000000001", importer.steamIDs[user.ID])
	}
	if _, ok := importer.xboxIDs[user.ID]; !ok {
		t.Errorf("Xbox import not triggered")
	}
}
