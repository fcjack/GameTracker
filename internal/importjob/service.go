package importjob

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jacksoncoelho/game-tracker/internal/cover"
	"github.com/jacksoncoelho/game-tracker/internal/crypto"
	"github.com/jacksoncoelho/game-tracker/internal/epic"
	"github.com/jacksoncoelho/game-tracker/internal/igdb"
	"github.com/jacksoncoelho/game-tracker/internal/metrics"
	"github.com/jacksoncoelho/game-tracker/internal/models"
	"github.com/jacksoncoelho/game-tracker/internal/playtime"
	"github.com/jacksoncoelho/game-tracker/internal/steam"
	"github.com/jacksoncoelho/game-tracker/internal/xbox"
)

const steamPlatform = "Steam"
const xboxPlatform = "Xbox"
const epicPlatform = "Epic"

func igdbConfigured() bool {
	return os.Getenv("TWITCH_CLIENT_ID") != "" && os.Getenv("TWITCH_CLIENT_SECRET") != ""
}

type Service struct {
	db        *pgxpool.Pool
	igdb      *igdb.Client
	steam     *steam.Client
	store     *steam.StoreClient
	xbox      *xbox.Client
	epic      *epic.Client
	encrypter *crypto.Encrypter
	covers    *cover.Resolver
	playtime  playtime.Publisher
}

func NewService(db *pgxpool.Pool, igdbClient *igdb.Client, encrypter *crypto.Encrypter) *Service {
	svc := NewServiceWithProviders(
		db,
		igdbClient,
		steam.NewClient(os.Getenv("STEAM_API_KEY")),
		nil,
		xbox.NewClient(os.Getenv("XBOX_CLIENT_ID"), os.Getenv("XBOX_CLIENT_SECRET")),
		encrypter,
	)
	svc.epic = epic.NewClient(os.Getenv("EPIC_CLIENT_ID"), os.Getenv("EPIC_CLIENT_SECRET"))
	return svc
}

func NewServiceWithSteam(db *pgxpool.Pool, igdbClient *igdb.Client, steamClient *steam.Client, storeClient *steam.StoreClient) *Service {
	return NewServiceWithProviders(db, igdbClient, steamClient, storeClient, nil, nil)
}

func NewServiceWithProviders(
	db *pgxpool.Pool,
	igdbClient *igdb.Client,
	steamClient *steam.Client,
	storeClient *steam.StoreClient,
	xboxClient *xbox.Client,
	encrypter *crypto.Encrypter,
) *Service {
	if storeClient == nil {
		storeClient = steam.NewStoreClient()
	}
	return &Service{
		db:        db,
		igdb:      igdbClient,
		steam:     steamClient,
		store:     storeClient,
		xbox:      xboxClient,
		encrypter: encrypter,
		covers:    cover.NewResolver(db, igdbClient),
	}
}

// SetEpicClient wires the Epic Games OAuth/library client. Used by tests.
func (s *Service) SetEpicClient(client *epic.Client) {
	s.epic = client
}

// SetPlaytimePublisher wires the background playtime worker pool.
func (s *Service) SetPlaytimePublisher(publisher playtime.Publisher) {
	s.playtime = publisher
}

func (s *Service) publishSteamPlaytime(userID int64, appID, minutes int) {
	if s.playtime == nil || appID <= 0 || minutes < 0 {
		return
	}
	s.playtime.Publish(playtime.Event{
		Kind:    playtime.KindSteam,
		UserID:  userID,
		AppID:   appID,
		Minutes: minutes,
	})
}

func (s *Service) publishXboxPlaytime(userID int64, game xbox.OwnedGame) {
	if s.playtime == nil || game.TitleID <= 0 {
		return
	}
	s.playtime.Publish(playtime.Event{
		Kind:    playtime.KindXbox,
		UserID:  userID,
		TitleID: game.TitleID,
		SCID:    game.SCID,
		Name:    game.Name,
	})
}

func (s *Service) StartSteamImport(_ context.Context, userID int64, steamID string) (*models.ImportJob, error) {
	ctx := context.Background()

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
	const provider = "steam"

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

	owned, err := s.steam.GetOwnedGames(steamID)
	if err != nil {
		fail(err.Error())
		return
	}

	if err := models.SetImportJobTotal(ctx, s.db, jobID, len(owned)); err != nil {
		fail("Failed to update import progress")
		return
	}

	games, err := s.store.FilterImportableGames(ctx, owned)
	if err != nil {
		fail("Failed to classify Steam apps: " + err.Error())
		return
	}

	hasIGDB := igdbConfigured()

	alreadyImported, err := models.ListImportedSteamAppIDs(ctx, s.db, userID, steamPlatform)
	if err != nil {
		fail("Failed to load existing library")
		return
	}

	var processed, imported, skipped int
	processed = len(owned) - len(games)
	skipped = processed
	if processed > 0 {
		if err := models.UpdateImportJobProgress(ctx, s.db, jobID, processed, imported, skipped); err != nil {
			slog.Warn("import job progress update failed",
				"provider", provider,
				"job_id", jobID,
				"error", err,
			)
		}
	}

	for _, g := range games {
		if importJobCancelled(ctx, s.db, jobID) {
			return
		}
		processed++

		if _, exists := alreadyImported[g.AppID]; exists {
			skipped++
			s.publishSteamPlaytime(userID, g.AppID, g.PlaytimeForever)
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
			igdbID, lookupErr = s.lookupWithRetry(g.AppID, g.Name)
			if lookupErr != nil {
				fail("IGDB lookup failed: " + lookupErr.Error())
				return
			}
		}

		added, err := s.importSteamGame(ctx, userID, g, igdbID)
		if err != nil {
			fail("Failed to import game: " + err.Error())
			return
		}
		s.publishSteamPlaytime(userID, g.AppID, g.PlaytimeForever)
		if added {
			imported++
			alreadyImported[g.AppID] = struct{}{}
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

func (s *Service) importSteamGame(ctx context.Context, userID int64, g steam.OwnedGame, igdbID int64) (bool, error) {
	if igdbID != 0 {
		added, handled, err := s.importIGDBGameWithRetry(ctx, userID, g, igdbID)
		if err != nil {
			return false, err
		}
		if handled {
			return added, nil
		}
	}
	return s.importSteamOnlyGame(ctx, userID, g)
}

func (s *Service) importIGDBGameWithRetry(ctx context.Context, userID int64, g steam.OwnedGame, igdbID int64) (added, handled bool, err error) {
	const maxAttempts = 3
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		added, handled, err = s.importIGDBGame(ctx, userID, g, igdbID)
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

func (s *Service) importIGDBGame(ctx context.Context, userID int64, g steam.OwnedGame, igdbID int64) (bool, bool, error) {
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

	added, err := s.persistIGDBGame(ctx, userID, g, igdbID, gameData)
	return added, true, err
}

func (s *Service) persistIGDBGame(ctx context.Context, userID int64, g steam.OwnedGame, igdbID int64, gameData *igdb.SearchResult) (bool, error) {
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

	game, err := models.ResolveGameForSteamImport(
		ctx, s.db, g.AppID, igdbID,
		g.Name, steam.CoverImageURL(g.AppID, g.ImgIconURL),
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
	coverURL := steam.CoverImageURL(g.AppID, g.ImgIconURL)
	if err := models.ApplySteamImportMetadata(ctx, s.db, game.ID, g.Name, coverURL, igdbCover); err != nil {
		return false, err
	}

	s.fetchCover(ctx, game.ID)

	playtime := g.PlaytimeForever
	return s.addGameToLibraryIfNeeded(ctx, userID, game.ID, steamPlatform, &playtime)
}

func (s *Service) importSteamOnlyGame(ctx context.Context, userID int64, g steam.OwnedGame) (bool, error) {
	cat, err := models.GetCategoryByIGDBValue(ctx, s.db, 0)
	if err != nil {
		return false, err
	}

	coverURL := steam.CoverImageURL(g.AppID, g.ImgIconURL)
	game, err := models.FindOrCreateGameBySteamAppID(ctx, s.db, g.AppID, g.Name, coverURL, cat.ID)
	if err != nil {
		return false, err
	}

	if err := models.ApplySteamImportMetadata(ctx, s.db, game.ID, g.Name, coverURL, ""); err != nil {
		return false, err
	}

	s.fetchCover(ctx, game.ID)

	playtime := g.PlaytimeForever
	return s.addGameToLibraryIfNeeded(ctx, userID, game.ID, steamPlatform, &playtime)
}

func (s *Service) fetchCover(ctx context.Context, gameID int64) {
	go func() {
		fetchCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.covers.FetchAndStore(fetchCtx, gameID); err != nil {
			slog.Warn("cover fetch failed", "game_id", gameID, "error", err)
		}
	}()
}

func (s *Service) addGameToLibraryIfNeeded(ctx context.Context, userID, gameID int64, platform string, playtimeMinutes *int) (bool, error) {
	exists, err := models.LibraryEntryExists(ctx, s.db, userID, gameID)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}

	if err := models.AddToLibrary(ctx, s.db, userID, gameID, platform, playtimeMinutes); err != nil {
		return false, err
	}
	return true, nil
}
