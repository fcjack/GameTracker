package models

import (
	"context"
	"os"
	"testing"

	"github.com/jacksoncoelho/game-tracker/internal/database"
)

func TestImportJobNullErrorMessage(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://gametracker:gametracker@localhost:5432/gametracker?sslmode=disable"
	}

	db, err := database.Connect(dbURL)
	if err != nil {
		t.Skipf("database not available: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	const userID int64 = 1

	job, err := GetLatestImportJob(ctx, db, userID, "steam")
	if err != nil {
		t.Fatalf("GetLatestImportJob() error = %v", err)
	}
	if job.ErrorMessage != "" && job.Status != "failed" {
		t.Fatalf("expected empty error_message for non-failed job, got %q", job.ErrorMessage)
	}
}
