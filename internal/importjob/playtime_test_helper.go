package importjob

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jacksoncoelho/game-tracker/internal/crypto"
	"github.com/jacksoncoelho/game-tracker/internal/igdb"
	"github.com/jacksoncoelho/game-tracker/internal/playtime"
	"github.com/jacksoncoelho/game-tracker/internal/steam"
	"github.com/jacksoncoelho/game-tracker/internal/xbox"
)

// attachSyncPlaytimePublisher processes playtime events inline during tests.
func attachSyncPlaytimePublisher(svc *Service, db *pgxpool.Pool, xboxClient *xbox.Client, enc *crypto.Encrypter) {
	if svc == nil {
		return
	}
	handler := playtime.NewHandler(db, xboxClient, enc)
	svc.SetPlaytimePublisher(playtime.NewSyncPublisher(handler))
}

func newXboxImportService(db *pgxpool.Pool, igdbClient *igdb.Client, xboxClient *xbox.Client, enc *crypto.Encrypter) *Service {
	svc := NewServiceWithProviders(db, igdbClient, nil, nil, xboxClient, enc)
	attachSyncPlaytimePublisher(svc, db, xboxClient, enc)
	return svc
}

func newXboxImportServiceAsync(db *pgxpool.Pool, igdbClient *igdb.Client, xboxClient *xbox.Client, enc *crypto.Encrypter) (*Service, *playtime.WorkerPool) {
	svc := NewServiceWithProviders(db, igdbClient, nil, nil, xboxClient, enc)
	handler := playtime.NewHandler(db, xboxClient, enc)
	pool := playtime.NewWorkerPool(handler, 2, 64)
	svc.SetPlaytimePublisher(pool)
	go pool.Run(context.Background())
	return svc, pool
}

func waitForPlaytimeDrain(t *testing.T, pool *playtime.WorkerPool, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if pool.Pending() == 0 {
			time.Sleep(100 * time.Millisecond)
			if pool.Pending() == 0 {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("playtime queue not drained within %s (pending=%d)", timeout, pool.Pending())
}

func newSteamImportService(db *pgxpool.Pool, igdbClient *igdb.Client, steamClient *steam.Client, storeClient *steam.StoreClient) *Service {
	svc := NewServiceWithSteam(db, igdbClient, steamClient, storeClient)
	handler := playtime.NewHandler(db, nil, nil)
	svc.SetPlaytimePublisher(playtime.NewSyncPublisher(handler))
	return svc
}
