package importjob

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jacksoncoelho/game-tracker/internal/igdb"
	"github.com/jacksoncoelho/game-tracker/internal/models"
	"github.com/jacksoncoelho/game-tracker/internal/steam"
)

const steamPlatform = "Steam"

type Service struct {
	db    *pgxpool.Pool
	igdb  *igdb.Client
	steam *steam.Client
}

func NewService(db *pgxpool.Pool, igdbClient *igdb.Client) *Service {
	return &Service{
		db:    db,
		igdb:  igdbClient,
		steam: steam.NewClient(os.Getenv("STEAM_API_KEY")),
	}
}

func (s *Service) StartSteamImport(ctx context.Context, userID int64, steamID string) (*models.ImportJob, error) {
	ctx = context.Background()

	active, err := models.HasActiveImportJob(ctx, s.db, userID, "steam")
	if err != nil {
		return nil, err
	}
	if active {
		job, err := models.GetLatestImportJob(ctx, s.db, userID, "steam")
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

	job, err := models.CreateImportJob(ctx, s.db, userID, "steam")
	if err != nil {
		return nil, err
	}

	go s.runSteamImport(job.ID, userID, steamID)
	return job, nil
}

func (s *Service) runSteamImport(jobID, userID int64, steamID string) {
	ctx := context.Background()
	log.Printf("steam import job %d: started for user %d", jobID, userID)

	fail := func(msg string) {
		log.Printf("steam import job %d failed: %s", jobID, msg)
		_ = models.FailImportJob(ctx, s.db, jobID, msg)
	}

	if os.Getenv("TWITCH_CLIENT_ID") == "" || os.Getenv("TWITCH_CLIENT_SECRET") == "" {
		fail("IGDB credentials are not configured")
		return
	}

	games, err := s.steam.GetOwnedGames(steamID)
	if err != nil {
		fail(err.Error())
		return
	}

	if err := models.SetImportJobTotal(ctx, s.db, jobID, len(games)); err != nil {
		fail("Failed to update import progress")
		return
	}

	var processed, imported, skipped int

	for _, g := range games {
		processed++

		igdbID, err := s.lookupWithRetry(g.AppID, g.Name)
		if err != nil {
			fail("IGDB lookup failed: " + err.Error())
			return
		}
		if igdbID == 0 {
			skipped++
		} else {
			added, err := s.importGameWithRetry(ctx, userID, igdbID)
			if err != nil {
				fail("Failed to import game: " + err.Error())
				return
			}
			if added {
				imported++
			}
		}

		if err := models.UpdateImportJobProgress(ctx, s.db, jobID, processed, imported, skipped); err != nil {
			log.Printf("steam import job %d: progress update failed: %v", jobID, err)
		}
	}

	if err := models.CompleteImportJob(ctx, s.db, jobID); err != nil {
		log.Printf("steam import job %d: complete failed: %v", jobID, err)
	}
}

func (s *Service) lookupWithRetry(appID int, steamName string) (int64, error) {
	const maxAttempts = 3
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		igdbID, err := s.igdb.LookupIGDBIDBySteamAppID(appID, steamName)
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

func (s *Service) importGameWithRetry(ctx context.Context, userID, igdbID int64) (bool, error) {
	const maxAttempts = 3
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		added, err := s.importGame(ctx, userID, igdbID)
		if err == nil {
			return added, nil
		}
		lastErr = err
		if !strings.Contains(err.Error(), "rate limited") {
			return false, err
		}
	}

	return false, lastErr
}

func (s *Service) importGame(ctx context.Context, userID, igdbID int64) (bool, error) {
	gameData, err := s.igdb.GetGameByID(igdbID)
	if err != nil {
		return false, err
	}
	if gameData == nil {
		return false, nil
	}

	cat, err := models.GetCategoryByIGDBValue(ctx, s.db, gameData.Category)
	if err != nil {
		cat, err = models.GetCategoryByIGDBValue(ctx, s.db, 0)
		if err != nil {
			return false, err
		}
	}

	coverURL := ""
	if gameData.Cover != nil {
		coverURL = gameData.Cover.URL
	}

	platforms := make([]string, len(gameData.Platforms))
	for i, p := range gameData.Platforms {
		platforms[i] = p.Name
	}

	game, err := models.FindOrCreateGame(
		ctx, s.db,
		igdbID, gameData.Name, coverURL,
		igdb.ReleaseYear(gameData.FirstReleaseDate),
		platforms, cat.ID,
	)
	if err != nil {
		return false, err
	}

	inLibrary, err := models.IsInLibrary(ctx, s.db, userID, game.ID)
	if err != nil {
		return false, err
	}
	if inLibrary {
		return false, nil
	}

	if err := models.AddToLibrary(ctx, s.db, userID, game.ID, steamPlatform); err != nil {
		return false, err
	}
	return true, nil
}
