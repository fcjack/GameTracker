package importjob

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/jacksoncoelho/game-tracker/internal/epic"
	"github.com/jacksoncoelho/game-tracker/internal/igdb"
	"github.com/jacksoncoelho/game-tracker/internal/metrics"
	"github.com/jacksoncoelho/game-tracker/internal/models"
)

func (s *Service) StartEpicImport(_ context.Context, userID int64) (*models.ImportJob, error) {
	ctx := context.Background()

	active, err := models.HasActiveImportJob(ctx, s.db, userID, "epic")
	if err != nil {
		return nil, err
	}
	if active {
		job, err := models.GetLatestImportJob(ctx, s.db, userID, "epic")
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

	job, err := models.CreateImportJob(ctx, s.db, userID, "epic")
	if err != nil {
		return nil, err
	}

	go s.runEpicImport(job.ID, userID)
	return job, nil
}

func (s *Service) runEpicImport(jobID, userID int64) {
	const provider = "epic"

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

	if s.epic == nil || s.encrypter == nil {
		fail("Epic import is not configured")
		return
	}

	tokens, err := epic.EnsureFreshTokens(ctx, s.epic, s.encrypter, s.db, userID)
	if err != nil {
		fail(err.Error())
		return
	}

	owned, err := s.epic.GetLibrary(ctx, tokens.AccessToken)
	if err != nil {
		fail(err.Error())
		return
	}

	if err := models.SetImportJobTotal(ctx, s.db, jobID, len(owned)); err != nil {
		fail("Failed to update import progress")
		return
	}

	hasIGDB := igdbConfigured()

	alreadyImported, err := models.ListImportedEpicCatalogItemIDs(ctx, s.db, userID, epicPlatform)
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

		game := g

		if _, exists := alreadyImported[game.CatalogItemID]; exists {
			if hasIGDB && s.epicLibraryEntryActive(ctx, userID, game.CatalogItemID) {
				s.syncEpicGameFromIGDB(ctx, game)
			}
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
			igdbID, lookupErr = s.lookupEpicWithRetry(game.CatalogItemID, game.Name)
			if lookupErr != nil {
				fail("IGDB lookup failed: " + lookupErr.Error())
				return
			}
		}

		added, err := s.importEpicGame(ctx, userID, game, igdbID)
		if err != nil {
			fail("Failed to import game: " + err.Error())
			return
		}
		if added {
			imported++
			alreadyImported[game.CatalogItemID] = struct{}{}
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

func (s *Service) lookupEpicWithRetry(catalogItemID, epicName string) (int64, error) {
	const maxAttempts = 3
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		igdbID, err := s.igdb.LookupIGDBIDByEpicCatalogItemID(catalogItemID, epicName)
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

func (s *Service) importEpicGame(ctx context.Context, userID int64, g epic.OwnedGame, igdbID int64) (bool, error) {
	if igdbID != 0 {
		added, handled, err := s.importEpicIGDBGameWithRetry(ctx, userID, g, igdbID)
		if err != nil {
			return false, err
		}
		if handled {
			return added, nil
		}
	}
	return s.importEpicOnlyGame(ctx, userID, g)
}

func (s *Service) importEpicIGDBGameWithRetry(ctx context.Context, userID int64, g epic.OwnedGame, igdbID int64) (added, handled bool, err error) {
	const maxAttempts = 3
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		added, handled, err = s.importEpicIGDBGame(ctx, userID, g, igdbID)
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

func (s *Service) importEpicIGDBGame(ctx context.Context, userID int64, g epic.OwnedGame, igdbID int64) (bool, bool, error) {
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

	added, err := s.persistEpicIGDBGame(ctx, userID, g, igdbID, gameData)
	return added, true, err
}

func (s *Service) persistEpicIGDBGame(ctx context.Context, userID int64, g epic.OwnedGame, igdbID int64, gameData *igdb.SearchResult) (bool, error) {
	game, err := s.upsertEpicGameFromIGDB(ctx, g, igdbID, gameData)
	if err != nil {
		return false, err
	}

	s.fetchCover(ctx, game.ID)

	return s.addGameToLibraryIfNeeded(ctx, userID, game.ID, epicPlatform, nil)
}

func (s *Service) upsertEpicGameFromIGDB(
	ctx context.Context,
	g epic.OwnedGame,
	igdbID int64,
	gameData *igdb.SearchResult,
) (*models.Game, error) {
	cat, err := models.GetCategoryByIGDBValue(ctx, s.db, gameData.Category)
	if err != nil {
		cat, err = models.GetCategoryByIGDBValue(ctx, s.db, 0)
		if err != nil {
			return nil, err
		}
	}

	platforms := make([]string, len(gameData.Platforms))
	for i, p := range gameData.Platforms {
		platforms[i] = p.Name
	}

	releaseYear, err := s.resolveEpicReleaseYear(g, gameData)
	if err != nil {
		return nil, err
	}

	game, err := models.ResolveGameForEpicImport(
		ctx, s.db, g.CatalogItemID, igdbID,
		g.Name, g.ImageURL,
		releaseYear,
		platforms, cat.ID,
	)
	if err != nil {
		return nil, err
	}

	igdbCover := ""
	if gameData.Cover != nil {
		igdbCover = gameData.Cover.URL
	}
	if err := models.ApplyEpicImportMetadata(ctx, s.db, game.ID, g.Name, g.ImageURL, igdbCover); err != nil {
		return nil, err
	}

	return game, nil
}

func (s *Service) resolveEpicReleaseYear(g epic.OwnedGame, gameData *igdb.SearchResult) (int, error) {
	year := igdb.ReleaseYearFromResult(gameData)
	if year > 0 {
		return year, nil
	}
	return s.lookupIGDBYearByNameWithRetry(g.Name)
}

func (s *Service) tryUpsertEpicFromIGDBID(ctx context.Context, g epic.OwnedGame, igdbID int64) error {
	gameData, err := s.igdb.GetGameByID(igdbID)
	if err != nil {
		return err
	}
	if gameData == nil {
		return nil
	}
	if !namesMatch(g.Name, gameData.Name) {
		return nil
	}
	if !igdb.IsMainGame(gameData.Category) {
		return nil
	}
	_, err = s.upsertEpicGameFromIGDB(ctx, g, igdbID, gameData)
	return err
}

func (s *Service) syncEpicGameFromIGDB(ctx context.Context, g epic.OwnedGame) {
	existing, err := models.GetGameByEpicCatalogItemID(ctx, s.db, g.CatalogItemID)
	if err != nil || existing.ReleaseYear > 0 {
		return
	}

	igdbID, err := s.lookupEpicWithRetry(g.CatalogItemID, g.Name)
	if err != nil {
		slog.Warn("epic metadata enrich failed",
			"catalog_item_id", g.CatalogItemID,
			"name", g.Name,
			"error", err,
		)
		return
	}
	if igdbID != 0 {
		if err := s.tryUpsertEpicFromIGDBID(ctx, g, igdbID); err != nil {
			slog.Warn("epic metadata enrich failed",
				"catalog_item_id", g.CatalogItemID,
				"name", g.Name,
				"error", err,
			)
			return
		}
		existing, err = models.GetGameByEpicCatalogItemID(ctx, s.db, g.CatalogItemID)
		if err == nil && existing.ReleaseYear > 0 {
			s.fetchCover(ctx, existing.ID)
			return
		}
	}

	gameData, err := s.lookupIGDBByNameWithRetry(g.Name)
	if err != nil {
		slog.Warn("epic metadata enrich failed",
			"catalog_item_id", g.CatalogItemID,
			"name", g.Name,
			"error", err,
		)
		return
	}
	if gameData != nil {
		game, err := s.upsertEpicGameFromIGDB(ctx, g, gameData.ID, gameData)
		if err != nil {
			slog.Warn("epic metadata enrich failed",
				"catalog_item_id", g.CatalogItemID,
				"name", g.Name,
				"error", err,
			)
			return
		}
		s.fetchCover(ctx, game.ID)
		return
	}

	year, err := s.lookupIGDBYearByNameWithRetry(g.Name)
	if err != nil {
		slog.Warn("epic metadata enrich failed",
			"catalog_item_id", g.CatalogItemID,
			"name", g.Name,
			"error", err,
		)
		return
	}
	if year > 0 {
		if err := models.UpdateGameReleaseYearIfEmpty(ctx, s.db, existing.ID, year); err != nil {
			slog.Warn("epic metadata enrich failed",
				"catalog_item_id", g.CatalogItemID,
				"name", g.Name,
				"error", err,
			)
		}
	}
}

func (s *Service) importEpicOnlyGame(ctx context.Context, userID int64, g epic.OwnedGame) (bool, error) {
	gameData, err := s.lookupIGDBByNameWithRetry(g.Name)
	if err != nil {
		return false, err
	}
	if gameData != nil {
		return s.persistEpicIGDBGame(ctx, userID, g, gameData.ID, gameData)
	}

	cat, err := models.GetCategoryByIGDBValue(ctx, s.db, 0)
	if err != nil {
		return false, err
	}

	game, err := models.FindOrCreateGameByEpicCatalogItemID(ctx, s.db, g.CatalogItemID, g.Name, g.ImageURL, cat.ID)
	if err != nil {
		return false, err
	}

	if err := models.ApplyEpicImportMetadata(ctx, s.db, game.ID, g.Name, g.ImageURL, ""); err != nil {
		return false, err
	}

	if year, err := s.lookupIGDBYearByNameWithRetry(g.Name); err != nil {
		return false, err
	} else if year > 0 {
		if err := models.UpdateGameReleaseYearIfEmpty(ctx, s.db, game.ID, year); err != nil {
			return false, err
		}
	}

	s.fetchCover(ctx, game.ID)

	return s.addGameToLibraryIfNeeded(ctx, userID, game.ID, epicPlatform, nil)
}

func (s *Service) epicLibraryEntryActive(ctx context.Context, userID int64, catalogItemID string) bool {
	game, err := models.GetGameByEpicCatalogItemID(ctx, s.db, catalogItemID)
	if err != nil {
		return false
	}
	active, err := models.IsInLibrary(ctx, s.db, userID, game.ID)
	return err == nil && active
}
