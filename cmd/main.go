package main

import (
	"html/template"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/jacksoncoelho/game-tracker/internal/crypto"
	"github.com/jacksoncoelho/game-tracker/internal/database"
	"github.com/jacksoncoelho/game-tracker/internal/handlers"
	"github.com/jacksoncoelho/game-tracker/internal/igdb"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, reading from environment")
	}

	db, err := database.Connect(os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
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
				log.Fatalf("Migration failed: %v", err)
			}
		case "down":
			if err := database.RollbackLastMigration(db); err != nil {
				log.Fatalf("Rollback failed: %v", err)
			}
		default:
			log.Fatalf("Unknown migrate direction %q — use up or down", direction)
		}
		return
	}

	// Normal startup: run pending migrations then serve
	if err := database.RunMigrations(db); err != nil {
		log.Fatalf("Migrations failed: %v", err)
	}

	secret := os.Getenv("SESSION_SECRET")
	if secret == "" {
		log.Fatal("SESSION_SECRET is required")
	}

	encryptionKey := os.Getenv("ENCRYPTION_KEY")
	if encryptionKey == "" {
		log.Fatal("ENCRYPTION_KEY is required")
	}

	_, err = crypto.NewEncrypter(encryptionKey)
	if err != nil {
		log.Fatalf("Invalid ENCRYPTION_KEY: %v", err)
	}

	r := gin.Default()
	r.SetHTMLTemplate(loadTemplates("templates"))
	r.Static("/static", "./static")
	r.Use(sessions.Sessions("session", cookie.NewStore([]byte(secret))))

	auth := handlers.NewAuthHandler(db)
	profile := handlers.NewProfileHandler(db)
	steam := handlers.NewSteamHandler(db)

	igdbBaseURL := os.Getenv("IGDB_BASE_URL")
	if igdbBaseURL == "" {
		igdbBaseURL = "https://api.igdb.com/v4"
	}
	igdbClient := igdb.NewClient(
		os.Getenv("TWITCH_CLIENT_ID"),
		os.Getenv("TWITCH_CLIENT_SECRET"),
		igdbBaseURL,
	)
	library := handlers.NewLibraryHandler(db, igdbClient)

	r.GET("/", auth.HomePage)
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
		protected.POST("/profile/avatar", profile.UploadAvatar)
		protected.GET("/profile/avatar", profile.ServeAvatar)
		protected.GET("/auth/steam", steam.Initiate)
		protected.GET("/auth/steam/callback", steam.Callback)
		protected.GET("/library", library.LibraryPage)
		protected.GET("/library/games", library.LibraryGrid)
		protected.GET("/library/search", library.Search)
		protected.POST("/library/games", library.AddGame)
		protected.DELETE("/library/games/:game_id", library.RemoveGame)
		protected.POST("/library/games/:game_id/status", library.UpdateStatus)
	}

	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}

	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func loadTemplates(dir string) *template.Template {
	tmpl := template.New("")
	if err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		template.Must(tmpl.ParseFiles(path))
		return nil
	}); err != nil {
		log.Fatalf("load templates: %v", err)
	}
	return tmpl
}
