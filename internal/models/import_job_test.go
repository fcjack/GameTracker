package models

import (
	"context"
	"testing"
	"time"
)

func TestImportJobProgressPercent(t *testing.T) {
	tests := []struct {
		name string
		job  ImportJob
		want int
	}{
		{"zero total", ImportJob{TotalCount: 0, ProcessedCount: 5}, 0},
		{"half done", ImportJob{TotalCount: 100, ProcessedCount: 50}, 50},
		{"complete", ImportJob{TotalCount: 10, ProcessedCount: 10}, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.job.ProgressPercent(); got != tt.want {
				t.Errorf("ProgressPercent() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestImportJobIsActive(t *testing.T) {
	if !(&ImportJob{Status: "pending"}).IsActive() {
		t.Error("pending should be active")
	}
	if !(&ImportJob{Status: "running"}).IsActive() {
		t.Error("running should be active")
	}
	if (&ImportJob{Status: "completed"}).IsActive() {
		t.Error("completed should not be active")
	}
}

func TestImportJobNeedsRestart(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name string
		job  ImportJob
		want bool
	}{
		{
			name: "completed",
			job:  ImportJob{Status: "completed", UpdatedAt: now.Add(-time.Hour)},
			want: false,
		},
		{
			name: "pending never started",
			job:  ImportJob{Status: "pending", UpdatedAt: now},
			want: true,
		},
		{
			name: "running with recent progress",
			job:  ImportJob{Status: "running", TotalCount: 10, ProcessedCount: 4, UpdatedAt: now},
			want: false,
		},
		{
			name: "running stale",
			job:  ImportJob{Status: "running", TotalCount: 10, ProcessedCount: 4, UpdatedAt: now.Add(-3 * time.Minute)},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.job.NeedsRestart(); got != tt.want {
				t.Errorf("NeedsRestart() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestImportJobSummary(t *testing.T) {
	tests := []struct {
		name string
		job  ImportJob
		want string
	}{
		{
			name: "completed empty library",
			job:  ImportJob{Status: "completed"},
			want: "No games found in your Steam library.",
		},
		{
			name: "completed one import",
			job:  ImportJob{Status: "completed", ImportedCount: 1},
			want: "Imported 1 game from Steam",
		},
		{
			name: "completed with skips",
			job:  ImportJob{Status: "completed", ImportedCount: 3, SkippedCount: 2},
			want: "Imported 3 games from Steam (2 already imported)",
		},
		{
			name: "failed with message",
			job:  ImportJob{Status: "failed", ErrorMessage: "steam: timeout"},
			want: "steam: timeout",
		},
		{
			name: "running",
			job:  ImportJob{Status: "running"},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.job.Summary(); got != tt.want {
				t.Errorf("Summary() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestImportJobLifecycle(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	user, err := CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	job, err := CreateImportJob(ctx, db, user.ID, "steam")
	if err != nil {
		t.Fatalf("CreateImportJob() error = %v", err)
	}
	if job.Status != "pending" {
		t.Errorf("status = %q, want pending", job.Status)
	}
	if job.ErrorMessage != "" {
		t.Errorf("error_message = %q, want empty", job.ErrorMessage)
	}

	if err := SetImportJobTotal(ctx, db, job.ID, 3); err != nil {
		t.Fatalf("SetImportJobTotal() error = %v", err)
	}
	if err := UpdateImportJobProgress(ctx, db, job.ID, 2, 1, 1); err != nil {
		t.Fatalf("UpdateImportJobProgress() error = %v", err)
	}
	if err := CompleteImportJob(ctx, db, job.ID); err != nil {
		t.Fatalf("CompleteImportJob() error = %v", err)
	}

	got, err := GetImportJob(ctx, db, job.ID)
	if err != nil {
		t.Fatalf("GetImportJob() error = %v", err)
	}
	if got.Status != "completed" {
		t.Errorf("status = %q, want completed", got.Status)
	}
	if got.TotalCount != 3 || got.ProcessedCount != 2 || got.ImportedCount != 1 || got.SkippedCount != 1 {
		t.Errorf("counts = total %d processed %d imported %d skipped %d",
			got.TotalCount, got.ProcessedCount, got.ImportedCount, got.SkippedCount)
	}
	if got.CompletedAt == nil {
		t.Error("completed_at should be set")
	}
}

func TestHasActiveImportJob(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	user, err := CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	active, err := HasActiveImportJob(ctx, db, user.ID, "steam")
	if err != nil {
		t.Fatalf("HasActiveImportJob() error = %v", err)
	}
	if active {
		t.Fatal("expected no active job")
	}

	job, err := CreateImportJob(ctx, db, user.ID, "steam")
	if err != nil {
		t.Fatalf("CreateImportJob() error = %v", err)
	}

	active, err = HasActiveImportJob(ctx, db, user.ID, "steam")
	if err != nil {
		t.Fatalf("HasActiveImportJob() after create error = %v", err)
	}
	if !active {
		t.Fatal("expected active job after create")
	}

	if err := FailImportJob(ctx, db, job.ID, "done"); err != nil {
		t.Fatalf("FailImportJob() error = %v", err)
	}

	active, err = HasActiveImportJob(ctx, db, user.ID, "steam")
	if err != nil {
		t.Fatalf("HasActiveImportJob() after fail error = %v", err)
	}
	if active {
		t.Fatal("expected no active job after fail")
	}
}
