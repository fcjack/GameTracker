package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/jacksoncoelho/game-tracker/internal/i18n"
	"github.com/jacksoncoelho/game-tracker/internal/models"
)

func dashboardTestRouter(h *AuthHandler) *gin.Engine {
	r := gin.New()
	store := cookie.NewStore([]byte("test-session-secret-32-chars!!"))
	r.Use(sessions.Sessions("session", store))
	r.SetHTMLTemplate(loadAllTemplates(filepath.Join("templates")))
	r.GET("/dashboard/stats", h.DashboardStats)
	return r
}

func withUserSession(t *testing.T, r *gin.Engine, userID int64, username string) *httptest.ResponseRecorder {
	t.Helper()

	setup := r.Group("/session")
	setup.GET("/setup", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("user_id", userID)
		session.Set("username", username)
		_ = session.Save()
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/session/setup", nil)
	r.ServeHTTP(w, req)

	return w
}

func TestDashboardStatsRendersPlatformPlaytime(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := t.Context()
	user, err := models.CreateUser(ctx, db, fmt.Sprintf("dash_playtime_%d", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if _, err := models.UpsertLinkedAccount(ctx, db, user.ID, "steam", "76561198012345678", "Gamer", "", "", nil); err != nil {
		t.Fatalf("UpsertLinkedAccount() error = %v", err)
	}

	cat, err := models.GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}
	game, err := models.FindOrCreateGameBySteamAppID(ctx, db, 570, "Dota 2", "", cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGameBySteamAppID() error = %v", err)
	}
	playtime := 150
	if err := models.AddToLibrary(ctx, db, user.ID, game.ID, "Steam", &playtime); err != nil {
		t.Fatalf("AddToLibrary() error = %v", err)
	}

	h := NewAuthHandler(db, false)
	r := dashboardTestRouter(h)
	setupRecorder := withUserSession(t, r, user.ID, user.Username)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/dashboard/stats", nil)
	for _, cookie := range setupRecorder.Result().Cookies() {
		req.AddCookie(cookie)
	}
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("DashboardStats() status = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	wantLabel := i18n.FormatPlaytimeDHM("en", 150)
	if !strings.Contains(body, wantLabel) {
		t.Fatalf("response missing playtime %q\nbody:\n%s", wantLabel, body)
	}
	if !strings.Contains(body, "Playtime by platform") {
		t.Fatalf("response missing section title\nbody:\n%s", body)
	}
	if !strings.Contains(body, `id="dashboard-stats-panel"`) {
		t.Fatalf("response missing stats panel wrapper\nbody:\n%s", body)
	}
}

func TestDashboardStatsOmitsPlatformPlaytimeWithoutLinks(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := t.Context()
	user, err := models.CreateUser(ctx, db, fmt.Sprintf("dash_no_links_%d", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	h := NewAuthHandler(db, false)
	r := dashboardTestRouter(h)
	setupRecorder := withUserSession(t, r, user.ID, user.Username)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/dashboard/stats", nil)
	for _, cookie := range setupRecorder.Result().Cookies() {
		req.AddCookie(cookie)
	}
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("DashboardStats() status = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if strings.Contains(body, "Playtime by platform") {
		t.Fatalf("response should not include platform playtime section\nbody:\n%s", body)
	}
}
