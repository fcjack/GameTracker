package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/jacksoncoelho/game-tracker/internal/igdb"
	"github.com/jacksoncoelho/game-tracker/internal/models"
)

func libraryTestRouter(h *LibraryHandler) *gin.Engine {
	r := gin.New()
	store := cookie.NewStore([]byte("test-session-secret-32-chars!!"))
	r.Use(sessions.Sessions("session", store))
	r.SetHTMLTemplate(loadAllTemplates(filepath.Join("templates")))

	r.GET("/test/session", func(c *gin.Context) {
		userID, err := strconv.ParseInt(c.Query("user_id"), 10, 64)
		if err != nil {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		session := sessions.Default(c)
		session.Set("user_id", userID)
		session.Set("username", c.Query("username"))
		_ = session.Save()
		c.Status(http.StatusOK)
	})
	r.GET("/library/search", h.SearchLibrary)
	r.GET("/library/search/igdb", h.SearchIGDB)
	return r
}

func sessionCookies(t *testing.T, r *gin.Engine, userID int64, username string) []*http.Cookie {
	t.Helper()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/test/session?user_id=%d&username=%s", userID, username), nil)
	r.ServeHTTP(w, req)
	return w.Result().Cookies()
}

func TestSearchLibraryRendersMatchingGames(t *testing.T) {
	t.Parallel()

	db := testDB(t)
	defer db.Close()

	ctx := t.Context()
	user, err := models.CreateUser(ctx, db, fmt.Sprintf("lib_search_%d", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	cat, err := models.GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}

	game, err := models.FindOrCreateGame(ctx, db, 96000, "Searchable Game", "", 2022, []string{"PC"}, cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGame() error = %v", err)
	}
	if err := models.AddToLibrary(ctx, db, user.ID, game.ID, "PC", nil); err != nil {
		t.Fatalf("AddToLibrary() error = %v", err)
	}

	h := NewLibraryHandler(db, nil)
	r := libraryTestRouter(h)
	cookies := sessionCookies(t, r, user.ID, user.Username)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/library/search?q=searchable", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("SearchLibrary() status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Searchable Game") {
		t.Fatalf("response missing game name:\n%s", body)
	}
	if !strings.Contains(body, `id="library-game-`) {
		t.Fatalf("response missing game card:\n%s", body)
	}
}

func TestSearchLibraryEmptyQuery(t *testing.T) {
	t.Parallel()

	db := testDB(t)
	defer db.Close()

	ctx := t.Context()
	user, err := models.CreateUser(ctx, db, fmt.Sprintf("lib_empty_%d", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	h := NewLibraryHandler(db, nil)
	r := libraryTestRouter(h)
	cookies := sessionCookies(t, r, user.ID, user.Username)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/library/search", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("SearchLibrary() status = %d, want %d", w.Code, http.StatusOK)
	}
	if strings.Contains(w.Body.String(), "No games found") {
		t.Fatalf("empty query should not show no-results message")
	}
}

func TestSearchLibraryNoResults(t *testing.T) {
	t.Parallel()

	db := testDB(t)
	defer db.Close()

	ctx := t.Context()
	user, err := models.CreateUser(ctx, db, fmt.Sprintf("lib_nores_%d", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	h := NewLibraryHandler(db, nil)
	r := libraryTestRouter(h)
	cookies := sessionCookies(t, r, user.ID, user.Username)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/library/search?q=missing-game", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("SearchLibrary() status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "No games found") {
		t.Fatalf("expected no-results message, got:\n%s", w.Body.String())
	}
}

func TestSearchIGDBRendersResults(t *testing.T) {
	t.Parallel()

	const testToken = "test-access-token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/token":
			json.NewEncoder(w).Encode(map[string]any{
				"access_token": testToken,
				"expires_in":   3600,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/games":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `search "Portal"`) {
				t.Errorf("search body = %q, expected Portal query", string(body))
			}
			json.NewEncoder(w).Encode([]igdb.SearchResult{
				{
					ID:       4242,
					Name:     "Portal",
					Category: 0,
					Cover:    &igdb.Cover{URL: "//images.igdb.com/igdb/image/upload/t_thumb/co1abc.jpg"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	db := testDB(t)
	defer db.Close()

	ctx := t.Context()
	user, err := models.CreateUser(ctx, db, fmt.Sprintf("igdb_search_%d", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	client := igdb.NewClient("test-client", "test-secret", server.URL)
	client.SetTokenURL(server.URL + "/token")
	client.SetHTTPClient(server.Client())

	h := NewLibraryHandler(db, client)
	r := libraryTestRouter(h)
	cookies := sessionCookies(t, r, user.ID, user.Username)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/library/search/igdb?q="+url.QueryEscape("Portal"), nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("SearchIGDB() status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Portal") {
		t.Fatalf("response missing IGDB result:\n%s", body)
	}
	if !strings.Contains(body, "Add to Library") {
		t.Fatalf("response missing add button:\n%s", body)
	}
}
