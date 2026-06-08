package models

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ImportJob struct {
	ID             int64
	UserID         int64
	Provider       string
	Status         string
	TotalCount     int
	ProcessedCount int
	ImportedCount  int
	SkippedCount   int
	ErrorMessage   string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	CompletedAt    *time.Time
}

func CreateImportJob(ctx context.Context, db *pgxpool.Pool, userID int64, provider string) (*ImportJob, error) {
	const query = `
		INSERT INTO import_jobs (user_id, provider, status, created_at, updated_at)
		VALUES ($1, $2, 'pending', NOW(), NOW())
		RETURNING id, user_id, provider, status, total_count, processed_count,
		          imported_count, skipped_count, COALESCE(error_message, ''), created_at, updated_at, completed_at
	`
	var job ImportJob
	err := db.QueryRow(ctx, query, userID, provider).Scan(
		&job.ID, &job.UserID, &job.Provider, &job.Status,
		&job.TotalCount, &job.ProcessedCount, &job.ImportedCount, &job.SkippedCount,
		&job.ErrorMessage, &job.CreatedAt, &job.UpdatedAt, &job.CompletedAt,
	)
	return &job, err
}

func GetImportJob(ctx context.Context, db *pgxpool.Pool, jobID int64) (*ImportJob, error) {
	const query = `
		SELECT id, user_id, provider, status, total_count, processed_count,
		       imported_count, skipped_count, COALESCE(error_message, ''), created_at, updated_at, completed_at
		FROM import_jobs
		WHERE id = $1
	`
	var job ImportJob
	err := db.QueryRow(ctx, query, jobID).Scan(
		&job.ID, &job.UserID, &job.Provider, &job.Status,
		&job.TotalCount, &job.ProcessedCount, &job.ImportedCount, &job.SkippedCount,
		&job.ErrorMessage, &job.CreatedAt, &job.UpdatedAt, &job.CompletedAt,
	)
	if err != nil {
		return nil, err
	}
	return &job, nil
}

func GetLatestImportJob(ctx context.Context, db *pgxpool.Pool, userID int64, provider string) (*ImportJob, error) {
	const query = `
		SELECT id, user_id, provider, status, total_count, processed_count,
		       imported_count, skipped_count, COALESCE(error_message, ''), created_at, updated_at, completed_at
		FROM import_jobs
		WHERE user_id = $1 AND provider = $2
		ORDER BY created_at DESC
		LIMIT 1
	`
	var job ImportJob
	err := db.QueryRow(ctx, query, userID, provider).Scan(
		&job.ID, &job.UserID, &job.Provider, &job.Status,
		&job.TotalCount, &job.ProcessedCount, &job.ImportedCount, &job.SkippedCount,
		&job.ErrorMessage, &job.CreatedAt, &job.UpdatedAt, &job.CompletedAt,
	)
	if err != nil {
		return nil, err
	}
	return &job, nil
}

func HasActiveImportJob(ctx context.Context, db *pgxpool.Pool, userID int64, provider string) (bool, error) {
	const query = `
		SELECT EXISTS(
			SELECT 1 FROM import_jobs
			WHERE user_id = $1 AND provider = $2 AND status IN ('pending', 'running')
		)
	`
	var exists bool
	err := db.QueryRow(ctx, query, userID, provider).Scan(&exists)
	return exists, err
}

func UpdateImportJobProgress(
	ctx context.Context,
	db *pgxpool.Pool,
	jobID int64,
	processed, imported, skipped int,
) error {
	const query = `
		UPDATE import_jobs
		SET processed_count = $2, imported_count = $3, skipped_count = $4, updated_at = NOW()
		WHERE id = $1
	`
	_, err := db.Exec(ctx, query, jobID, processed, imported, skipped)
	return err
}

func SetImportJobTotal(ctx context.Context, db *pgxpool.Pool, jobID int64, total int) error {
	const query = `
		UPDATE import_jobs
		SET total_count = $2, status = 'running', updated_at = NOW()
		WHERE id = $1
	`
	_, err := db.Exec(ctx, query, jobID, total)
	return err
}

func CompleteImportJob(ctx context.Context, db *pgxpool.Pool, jobID int64) error {
	const query = `
		UPDATE import_jobs
		SET status = 'completed', completed_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`
	_, err := db.Exec(ctx, query, jobID)
	return err
}

func FailImportJob(ctx context.Context, db *pgxpool.Pool, jobID int64, message string) error {
	const query = `
		UPDATE import_jobs
		SET status = 'failed', error_message = $2, completed_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`
	_, err := db.Exec(ctx, query, jobID, message)
	return err
}

func (j *ImportJob) ProgressPercent() int {
	if j.TotalCount == 0 {
		return 0
	}
	return int(float64(j.ProcessedCount) / float64(j.TotalCount) * 100)
}

func (j *ImportJob) IsActive() bool {
	return j.Status == "pending" || j.Status == "running"
}

// NeedsRestart reports jobs that were interrupted before finishing.
func (j *ImportJob) NeedsRestart() bool {
	if !j.IsActive() {
		return false
	}
	if j.TotalCount == 0 && j.ProcessedCount == 0 {
		return true
	}
	// Active but no progress updates recently (e.g. server restarted mid-import).
	return time.Since(j.UpdatedAt) > 2*time.Minute
}

func (j *ImportJob) Summary() string {
	switch j.Status {
	case "completed":
		if j.ImportedCount == 0 && j.SkippedCount == 0 {
			return "No games found in your Steam library."
		}
		msg := fmt.Sprintf("Imported %d game", j.ImportedCount)
		if j.ImportedCount != 1 {
			msg += "s"
		}
		msg += " from Steam"
		if j.SkippedCount > 0 {
			msg += fmt.Sprintf(" (%d not matched in IGDB)", j.SkippedCount)
		}
		return msg
	case "failed":
		if j.ErrorMessage != "" {
			return j.ErrorMessage
		}
		return "Import failed."
	default:
		return ""
	}
}
