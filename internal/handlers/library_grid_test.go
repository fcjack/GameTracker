package handlers

import (
	"fmt"
	"html/template"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jacksoncoelho/game-tracker/internal/i18n"
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
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := t.Context()
	user, err := models.CreateUser(ctx, db, fmt.Sprintf("grid_%d", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	cat, err := models.GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}

	for i, name := range []string{"Alpha Game", "Beta Game", "Gamma Game"} {
		game, err := models.FindOrCreateGame(ctx, db, int64(90000+i), name, "", 2020, []string{"PC"}, cat.ID)
		if err != nil {
			t.Fatalf("FindOrCreateGame() error = %v", err)
		}
		if err := models.AddToLibrary(ctx, db, user.ID, game.ID, "PC", nil); err != nil {
			t.Fatalf("AddToLibrary() error = %v", err)
		}
	}

	games, err := models.ListUserGames(ctx, db, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 3 {
		t.Fatalf("setup: want 3 games, got %d", len(games))
	}

	tmpl := loadAllTemplates(filepath.Join("templates"))
	data := gin.H{
		"hasGames": true,
		"games":    toLibraryCardsWithLocale("en", games, true),
		"T":        i18n.NewTranslator("en"),
		"lang":     "en",
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
