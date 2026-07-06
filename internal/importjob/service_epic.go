package importjob

import (
	"context"

	"github.com/jacksoncoelho/game-tracker/internal/models"
)

// StartEpicImport is wired after account linking; library fetch is implemented in #41.
func (s *Service) StartEpicImport(_ context.Context, _ int64) (*models.ImportJob, error) {
	return nil, nil
}
