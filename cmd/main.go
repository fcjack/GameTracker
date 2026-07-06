package main

import (
	"context"
	"html/template"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/jacksoncoelho/game-tracker/internal/config"
	"github.com/jacksoncoelho/game-tracker/internal/crypto"
	"github.com/jacksoncoelho/game-tracker/internal/database"
	"github.com/jacksoncoelho/game-tracker/internal/handlers"
	"github.com/jacksoncoelho/game-tracker/internal/igdb"
	"github.com/jacksoncoelho/game-tracker/internal/importjob"
	"github.com/jacksoncoelho/game-tracker/internal/logging"
	"github.com/jacksoncoelho/game-tracker/internal/metrics"
	"github.com/jacksoncoelho/game-tracker/internal/playtime"
	"github.com/jacksoncoelho/game-tracker/internal/xbox"
	"github.com/joho/godotenv"
)

func main() {
	logger := logging.Init()

	if err := godotenv.Load(); err != nil {
		logger.Info("no .env file found, reading from environment")
	}

	db, err := database.Connect(os.Getenv("DATABASE_URL"))
	if err != nil {
		logger.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Handle migrate subcommands and exit without starting the server
	if len(os.Args) >= 2 && os.Args[1] == "migrate" {
		direction := "up"
		if len(os.Args) >= 3 {
			direction = os.Args[2]
		}
		switch direction {
		case "up":
			if err := database.RunMigrations(db); err != nil {
				logger.Error("migration failed", "error", err)
				os.Exit(1)
			}
		case "down":
			if err := database.RollbackLastMigration(db); err != nil {
				logger.Error("rollback failed", "error", err)
				os.Exit(1)
			}
		default:
			logger.Error("unknown migrate direction", "direction", direction)
			os.Exit(1)
		}
		return
	}

	// Normal startup: run pending migrations then serve
	if err := database.RunMigrations(db); err != nil {
		logger.Error("migrations failed", "error", err)
		os.Exit(1)
	}

	secret := os.Getenv("SESSION_SECRET")
	if secret == "" {
		logger.Error("SESSION_SECRET is required")
		os.Exit(1)
	}

	encryptionKey := os.Getenv("ENCRYPTION_KEY")
	if encryptionKey == "" {
		logger.Error("ENCRYPTION_KEY is required")
		os.Exit(1)
	}

	encrypter, err := crypto.NewEncrypter(encryptionKey)
	if err != nil {
		logger.Error("invalid ENCRYPTION_KEY", "error", err)
		os.Exit(1)
	}

	r := gin.New()
	r.Use(handlers.Recovery())
	r.Use(handlers.RequestMetrics())
	r.Use(handlers.RequestLogger())
	r.GET("/healthz", func(c *gin.Context) {
		if err := db.Ping(c.Request.Context()); err != nil {
			c.JSON(503, gin.H{"status": "unhealthy"})
			return
		}
		c.JSON(200, gin.H{"status": "ok"})
	})
	r.GET("/metrics", gin.WrapH(metrics.Handler()))
	r.SetHTMLTemplate(loadTemplates("templates"))
	// Static assets are referenced with a ?v=<version> query, so they can be
	// cached aggressively and still bust on release.
	static := r.Group("/static", func(c *gin.Context) {
		c.Header("Cache-Control", "public, max-age=86400")
	})
	static.Static("/", "./static")
	r.GET("/library/cover-placeholder", handlers.ServeCoverPlaceholder)
	r.GET("/service-worker.js", func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache")
		c.File("./static/service-worker.js")
	})
	r.Use(sessions.Sessions("session", cookie.NewStore([]byte(secret))))
	r.Use(handlers.LocaleMiddleware(db))

	auth := handlers.NewAuthHandler(db, config.RegistrationEnabled())
	profile := handlers.NewProfileHandler(db)

	igdbBaseURL := os.Getenv("IGDB_BASE_URL")
	if igdbBaseURL == "" {
		igdbBaseURL = "https://api.igdb.com/v4"
	}
	igdbClient := igdb.NewClient(
		os.Getenv("TWITCH_CLIENT_ID"),
		os.Getenv("TWITCH_CLIENT_SECRET"),
		igdbBaseURL,
	)
	importService := importjob.NewService(db, igdbClient, encrypter)

	xboxClient := xbox.NewClient(os.Getenv("XBOX_CLIENT_ID"), os.Getenv("XBOX_CLIENT_SECRET"))
	playtimeHandler := playtime.NewHandler(db, xboxClient, encrypter)
	playtimePool := playtime.NewWorkerPool(
		playtimeHandler,
		config.PlaytimeWorkerCount(),
		config.PlaytimeQueueSize(),
	)
	importService.SetPlaytimePublisher(playtimePool)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go playtimePool.Run(ctx)
	logger.Info("playtime worker pool started",
		"workers", config.PlaytimeWorkerCount(),
		"queue_size", config.PlaytimeQueueSize(),
		"rate_per_second", config.PlaytimeRatePerSecond(),
	)

	if config.LibrarySyncEnabled() {
		interval := config.LibrarySyncInterval()
		scheduler := importjob.NewScheduler(db, importService, interval)
		go scheduler.Run(context.Background())
		logger.Info("scheduled library sync enabled", "interval", interval.String())
	} else {
		logger.Info("scheduled library sync disabled")
	}

	steam := handlers.NewSteamHandler(db, importService)
	xboxHandler := handlers.NewXboxHandler(db, encrypter, importService)
	epicHandler := handlers.NewEpicHandler(db, encrypter, importService)
	importHandler := handlers.NewImportHandler(db, importService)
	library := handlers.NewLibraryHandler(db, igdbClient)

	r.GET("/", auth.LoginPage)
	r.GET("/login", auth.LoginPage)
	r.POST("/login", auth.Login)
	r.GET("/register", auth.RegisterPage)
	r.POST("/register", auth.Register)
	r.POST("/logout", auth.Logout)

	protected := r.Group("/")
	protected.Use(handlers.AuthRequired())
	{
		protected.GET("/dashboard", auth.Dashboard)
		protected.GET("/dashboard/stats", auth.DashboardStats)
		protected.GET("/profile", profile.ProfilePage)
		protected.POST("/profile/locale", profile.ChangeLocale)
		protected.POST("/profile/avatar", profile.UploadAvatar)
		protected.POST("/profile/password", profile.ChangePassword)
		protected.GET("/profile/avatar", profile.ServeAvatar)
		protected.GET("/auth/steam", steam.Initiate)
		protected.GET("/auth/steam/callback", steam.Callback)
		protected.GET("/auth/xbox", xboxHandler.Initiate)
		protected.GET("/auth/xbox/callback", xboxHandler.Callback)
		protected.GET("/auth/epic", epicHandler.Initiate)
		protected.GET("/auth/epic/callback", epicHandler.Callback)
		protected.POST("/profile/steam/import", importHandler.StartSteamImport)
		protected.POST("/profile/steam/clear-library", importHandler.ClearSteamLibrary)
		protected.POST("/profile/steam/unlink", importHandler.UnlinkSteamAccount)
		protected.GET("/profile/steam/import-status", importHandler.SteamImportStatus)
		protected.POST("/profile/steam/import-cancel", importHandler.CancelSteamImport)
		protected.POST("/profile/xbox/import", importHandler.StartXboxImport)
		protected.POST("/profile/xbox/clear-library", importHandler.ClearXboxLibrary)
		protected.POST("/profile/xbox/unlink", importHandler.UnlinkXboxAccount)
		protected.GET("/profile/xbox/import-status", importHandler.XboxImportStatus)
		protected.POST("/profile/xbox/import-cancel", importHandler.CancelXboxImport)
		protected.GET("/library", library.LibraryPage)
		protected.GET("/library/games", library.LibraryGrid)
		protected.GET("/library/games/:game_id", library.GameDetail)
		protected.GET("/library/games/:game_id/cover", library.ServeGameCover)
		protected.GET("/library/search", library.SearchLibrary)
		protected.GET("/library/search/igdb", library.SearchIGDB)
		protected.GET("/library/games/:game_id/link-igdb-form", library.LinkIGDBForm)
		protected.GET("/library/games/:game_id/search-igdb", library.SearchIGDBForLink)
		protected.POST("/library/games/:game_id/link-igdb", library.LinkGameToIGDB)
		protected.POST("/library/games", library.AddGame)
		protected.DELETE("/library/games/:game_id", library.RemoveGame)
		protected.GET("/library/games/:game_id/complete-form", library.CompleteGameForm)
		protected.POST("/library/games/:game_id/complete", library.CompleteGame)
		protected.POST("/library/games/:game_id/playing", library.SetPlaying)
		protected.POST("/library/games/:game_id/dropped", library.SetDropped)
		protected.POST("/library/games/:game_id/status", library.UpdateStatus)
	}

	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}

	logger.Info("server starting", "port", port)
	if err := r.Run(":" + port); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func loadTemplates(dir string) *template.Template {
	logger := slog.Default()
	tmpl := template.New("")
	if err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		template.Must(tmpl.ParseFiles(path))
		return nil
	}); err != nil {
		logger.Error("load templates failed", "error", err)
		os.Exit(1)
	}
	return tmpl
}
