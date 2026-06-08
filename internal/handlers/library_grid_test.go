package handlers

import (
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jacksoncoelho/game-tracker/internal/database"
	"github.com/jacksoncoelho/game-tracker/internal/models"
)

func loadAllTemplates(dir string) *template.Template {
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

func TestLibraryGridRendersAllGames(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://gametracker:gametracker@localhost:5432/gametracker?sslmode=disable"
	}
	db, err := database.Connect(dbURL)
	if err != nil {
		t.Skip("database not available:", err)
	}
	defer db.Close()

	games, err := models.ListUserGames(t.Context(), db, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 3 {
		t.Fatalf("setup: want 3 games from db, got %d", len(games))
	}

	tmpl := loadAllTemplates(filepath.Join("..", "..", "templates"))
	data := gin.H{
		"hasGames": true,
		"games":    toLibraryCards(games, true),
	}
	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, "library/game_grid", data); err != nil {
		t.Fatal(err)
	}
	body := buf.String()
	count := strings.Count(body, `id="library-game-`)
	if count != 3 {
		t.Fatalf("want 3 cards, got %d\nbody:\n%s", count, body)
	}
}
