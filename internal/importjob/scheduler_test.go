package importjob

import (
	"context"
	"sync"
	"testing"

	"github.com/jacksoncoelho/game-tracker/internal/models"
)

type fakeImporter struct {
	mu      sync.Mutex
	calls   []int64
	steamID []string
}

func (f *fakeImporter) StartSteamImport(_ context.Context, userID int64, steamID string) (*models.ImportJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, userID)
	f.steamID = append(f.steamID, steamID)
	return &models.ImportJob{UserID: userID}, nil
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

	triggered := map[int64]string{}
	for i, uid := range importer.calls {
		triggered[uid] = importer.steamID[i]
	}

	if triggered[user1.ID] != "76561190000000001" {
		t.Errorf("user1 import steamID = %q, want 76561190000000001", triggered[user1.ID])
	}
	if triggered[user2.ID] != "76561190000000002" {
		t.Errorf("user2 import steamID = %q, want 76561190000000002", triggered[user2.ID])
	}
}
