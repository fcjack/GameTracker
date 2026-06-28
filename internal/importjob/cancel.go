package importjob

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jacksoncoelho/game-tracker/internal/models"
)

func (s *Service) CancelImport(ctx context.Context, userID int64, provider, message string) (bool, error) {
	return models.FailActiveImportJob(ctx, s.db, userID, provider, message)
}

func importJobCancelled(ctx context.Context, db *pgxpool.Pool, jobID int64) bool {
	active, err := models.IsImportJobActive(ctx, db, jobID)
	if err != nil {
		return true
	}
	return !active
}
