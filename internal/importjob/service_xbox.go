package importjob

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jacksoncoelho/game-tracker/internal/igdb"
	"github.com/jacksoncoelho/game-tracker/internal/metrics"
	"github.com/jacksoncoelho/game-tracker/internal/models"
	"github.com/jacksoncoelho/game-tracker/internal/xbox"
)

func (s *Service) StartXboxImport(_ context.Context, userID int64) (*models.ImportJob, error) {
	ctx := context.Background()

	active, err := models.HasActiveImportJob(ctx, s.db, userID, "xbox")
	if err != nil {
		return nil, err
	}
	if active {
		job, err := models.GetLatestImportJob(ctx, s.db, userID, "xbox")
		if err != nil {
			return nil, err
		}
		if !job.NeedsRestart() {
			return job, nil
		}
		if err := models.FailImportJob(ctx, s.db, job.ID, "Import interrupted, restarting"); err != nil {
			return nil, err
		}
	}

	job, err := models.CreateImportJob(ctx, s.db, userID, "xbox")
	if err != nil {
		return nil, err
	}

	go s.runXboxImport(job.ID, userID)
	return job, nil
}

func (s *Service) runXboxImport(jobID, userID int64) {
	const provider = "xbox"

	ctx := context.Background()
	start := time.Now()
	metrics.ImportJobsActive.WithLabelValues(provider).Inc()
	defer metrics.ImportJobsActive.WithLabelValues(provider).Dec()
	metrics.ImportJobsTotal.WithLabelValues(provider, "started").Inc()

	slog.Info("import job started",
		"provider", provider,
		"job_id", jobID,
		"user_id", userID,
	)

	fail := func(msg string) {
		duration := time.Since(start)
		metrics.ImportJobsTotal.WithLabelValues(provider, "failed").Inc()
		metrics.ImportJobDuration.WithLabelValues(provider).Observe(duration.Seconds())
		slog.Error("import job failed",
			"provider", provider,
			"job_id", jobID,
			"user_id", userID,
			"error", msg,
			"duration_ms", duration.Milliseconds(),
		)
		_ = models.FailImportJob(ctx, s.db, jobID, msg)
	}

	if s.xbox == nil || s.encrypter == nil {
		fail("Xbox import is not configured")
		return
	}

	tokens, err := xbox.EnsureFreshTokens(ctx, s.xbox, s.encrypter, s.db, userID)
	if err != nil {
		fail(err.Error())
		return
	}

	owned, err := s.xbox.GetOwnedGames(ctx, tokens.AccessToken)
	if err != nil {
		fail(err.Error())
		return
	}

	if err := models.SetImportJobTotal(ctx, s.db, jobID, len(owned)); err != nil {
		fail("Failed to update import progress")
		return
	}

	hasIGDB := os.Getenv("TWITCH_CLIENT_ID") != "" && os.Getenv("TWITCH_CLIENT_SECRET") != ""

	alreadyImported, err := models.ListImportedXboxTitleIDs(ctx, s.db, userID, xboxPlatform)
	if err != nil {
		fail("Failed to load existing library")
		return
	}

	var processed, imported, skipped int
	for _, g := range owned {
		if importJobCancelled(ctx, s.db, jobID) {
			return
		}
		processed++

		if _, exists := alreadyImported[g.TitleID]; exists {
			skipped++
			if err := models.UpdateImportJobProgress(ctx, s.db, jobID, processed, imported, skipped); err != nil {
				slog.Warn("import job progress update failed",
					"provider", provider,
					"job_id", jobID,
					"error", err,
				)
			}
			continue
		}

		var igdbID int64
		if hasIGDB {
			var lookupErr error
			igdbID, lookupErr = s.lookupXboxWithRetry(g.TitleID, g.Name)
			if lookupErr != nil {
				fail("IGDB lookup failed: " + lookupErr.Error())
				return
			}
		}

		added, err := s.importXboxGame(ctx, userID, g, igdbID)
		if err != nil {
			fail("Failed to import game: " + err.Error())
			return
		}
		if added {
			imported++
			alreadyImported[g.TitleID] = struct{}{}
		} else {
			skipped++
		}

		if err := models.UpdateImportJobProgress(ctx, s.db, jobID, processed, imported, skipped); err != nil {
			slog.Warn("import job progress update failed",
				"provider", provider,
				"job_id", jobID,
				"error", err,
			)
		}
	}

	if importJobCancelled(ctx, s.db, jobID) {
		return
	}

	if err := models.CompleteImportJob(ctx, s.db, jobID); err != nil {
		slog.Error("import job complete failed",
			"provider", provider,
			"job_id", jobID,
			"error", err,
		)
		return
	}

	duration := time.Since(start)
	metrics.ImportJobsTotal.WithLabelValues(provider, "completed").Inc()
	metrics.ImportJobDuration.WithLabelValues(provider).Observe(duration.Seconds())
	metrics.ImportGamesTotal.WithLabelValues(provider, "imported").Add(float64(imported))
	metrics.ImportGamesTotal.WithLabelValues(provider, "skipped").Add(float64(skipped))
	slog.Info("import job completed",
		"provider", provider,
		"job_id", jobID,
		"user_id", userID,
		"processed", processed,
		"imported", imported,
		"skipped", skipped,
		"duration_ms", duration.Milliseconds(),
	)
}

func (s *Service) lookupXboxWithRetry(titleID int, xboxName string) (int64, error) {
	const maxAttempts = 3
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		igdbID, err := s.igdb.LookupIGDBIDByXboxTitleID(titleID, xboxName)
		if err == nil {
			return igdbID, nil
		}
		lastErr = err
		if !strings.Contains(err.Error(), "rate limited") {
			return 0, err
		}
	}

	return 0, lastErr
}

func (s *Service) importXboxGame(ctx context.Context, userID int64, g xbox.OwnedGame, igdbID int64) (bool, error) {
	if igdbID != 0 {
		added, handled, err := s.importXboxIGDBGameWithRetry(ctx, userID, g, igdbID)
		if err != nil {
			return false, err
		}
		if handled {
			return added, nil
		}
	}
	return s.importXboxOnlyGame(ctx, userID, g)
}

func (s *Service) importXboxIGDBGameWithRetry(ctx context.Context, userID int64, g xbox.OwnedGame, igdbID int64) (added, handled bool, err error) {
	const maxAttempts = 3
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		added, handled, err = s.importXboxIGDBGame(ctx, userID, g, igdbID)
		if err == nil {
			return added, handled, nil
		}
		lastErr = err
		if !strings.Contains(err.Error(), "rate limited") {
			return false, false, err
		}
	}

	return false, false, lastErr
}

func (s *Service) importXboxIGDBGame(ctx context.Context, userID int64, g xbox.OwnedGame, igdbID int64) (bool, bool, error) {
	gameData, err := s.igdb.GetGameByID(igdbID)
	if err != nil {
		return false, false, err
	}
	if gameData == nil {
		return false, false, nil
	}
	if !namesMatch(g.Name, gameData.Name) {
		return false, false, nil
	}
	if !igdb.IsMainGame(gameData.Category) {
		return false, false, nil
	}

	added, err := s.persistXboxIGDBGame(ctx, userID, g, igdbID, gameData)
	return added, true, err
}

func (s *Service) persistXboxIGDBGame(ctx context.Context, userID int64, g xbox.OwnedGame, igdbID int64, gameData *igdb.SearchResult) (bool, error) {
	cat, err := models.GetCategoryByIGDBValue(ctx, s.db, gameData.Category)
	if err != nil {
		cat, err = models.GetCategoryByIGDBValue(ctx, s.db, 0)
		if err != nil {
			return false, err
		}
	}

	platforms := make([]string, len(gameData.Platforms))
	for i, p := range gameData.Platforms {
		platforms[i] = p.Name
	}

	game, err := models.ResolveGameForXboxImport(
		ctx, s.db, g.TitleID, igdbID,
		g.Name, g.ImageURL,
		igdb.ReleaseYear(gameData.FirstReleaseDate),
		platforms, cat.ID,
	)
	if err != nil {
		return false, err
	}

	igdbCover := ""
	if gameData.Cover != nil {
		igdbCover = gameData.Cover.URL
	}
	if err := models.ApplyXboxImportMetadata(ctx, s.db, game.ID, g.Name, g.ImageURL, igdbCover); err != nil {
		return false, err
	}

	s.fetchCover(ctx, game.ID)

	return s.addGameToLibraryIfNeeded(ctx, userID, game.ID, xboxPlatform, nil)
}

func (s *Service) importXboxOnlyGame(ctx context.Context, userID int64, g xbox.OwnedGame) (bool, error) {
	cat, err := models.GetCategoryByIGDBValue(ctx, s.db, 0)
	if err != nil {
		return false, err
	}

	game, err := models.FindOrCreateGameByXboxTitleID(ctx, s.db, g.TitleID, g.Name, g.ImageURL, cat.ID)
	if err != nil {
		return false, err
	}

	if err := models.ApplyXboxImportMetadata(ctx, s.db, game.ID, g.Name, g.ImageURL, ""); err != nil {
		return false, err
	}

	s.fetchCover(ctx, game.ID)

	return s.addGameToLibraryIfNeeded(ctx, userID, game.ID, xboxPlatform, nil)
}
