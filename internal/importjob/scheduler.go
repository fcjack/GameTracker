package importjob

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jacksoncoelho/game-tracker/internal/models"
)

// steamImporter is the subset of *Service the scheduler needs, kept small for testing.
type steamImporter interface {
	StartSteamImport(ctx context.Context, userID int64, steamID string) (*models.ImportJob, error)
}

// Scheduler periodically re-syncs linked libraries for all users on a fixed interval.
// It reuses the existing import job machinery, which is idempotent and guards against
// overlapping runs, so scheduled and manual imports never collide.
type Scheduler struct {
	db       *pgxpool.Pool
	importer steamImporter
	interval time.Duration
}

func NewScheduler(db *pgxpool.Pool, importer steamImporter, interval time.Duration) *Scheduler {
	return &Scheduler{db: db, importer: importer, interval: interval}
}

// Run blocks until ctx is cancelled, triggering a sync on every interval tick.
func (s *Scheduler) Run(ctx context.Context) {
	slog.Info("library sync scheduler started", "interval", s.interval.String())

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("library sync scheduler stopped")
			return
		case <-ticker.C:
			s.SyncSteam(ctx)
		}
	}
}

// SyncSteam triggers a Steam import for every user with a linked Steam account.
func (s *Scheduler) SyncSteam(ctx context.Context) {
	accounts, err := models.ListLinkedAccountsByProvider(ctx, s.db, "steam")
	if err != nil {
		slog.Error("scheduled sync: failed to list linked Steam accounts", "error", err)
		return
	}

	slog.Info("scheduled Steam sync starting", "accounts", len(accounts))

	var triggered int
	for _, acct := range accounts {
		if ctx.Err() != nil {
			return
		}
		if _, err := s.importer.StartSteamImport(ctx, acct.UserID, acct.ExternalID); err != nil {
			slog.Error("scheduled sync: failed to start Steam import",
				"user_id", acct.UserID,
				"error", err,
			)
			continue
		}
		triggered++
	}

	slog.Info("scheduled Steam sync dispatched", "triggered", triggered, "total", len(accounts))
}
