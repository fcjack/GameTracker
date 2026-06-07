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
	"github.com/jacksoncoelho/game-tracker/internal/database"
	"github.com/jacksoncoelho/game-tracker/internal/handlers"
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

	if err := database.RunMigrations(db); err != nil {
		log.Fatalf("Migrations failed: %v", err)
	}

	r := gin.Default()
	r.SetHTMLTemplate(loadTemplates("templates"))
	r.Static("/static", "./static")

	secret := os.Getenv("SESSION_SECRET")
	if secret == "" {
		log.Fatal("SESSION_SECRET is required")
	}
	r.Use(sessions.Sessions("session", cookie.NewStore([]byte(secret))))

	auth := handlers.NewAuthHandler(db)

	r.GET("/login", auth.LoginPage)
	r.POST("/login", auth.Login)
	r.GET("/register", auth.RegisterPage)
	r.POST("/register", auth.Register)
	r.POST("/logout", auth.Logout)

	protected := r.Group("/")
	protected.Use(handlers.AuthRequired())
	{
		protected.GET("/", auth.Dashboard)
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
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		template.Must(tmpl.ParseFiles(path))
		return nil
	})
	return tmpl
}
