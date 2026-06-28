package handlers

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/jacksoncoelho/game-tracker/internal/igdb"
	"github.com/jacksoncoelho/game-tracker/internal/importjob"
	"github.com/jacksoncoelho/game-tracker/internal/models"
	"github.com/jacksoncoelho/game-tracker/internal/steam"
	"github.com/jacksoncoelho/game-tracker/internal/xbox"
)

func TestStartSteamImportNotLinked(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := t.Context()
	user, err := models.CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	svc := importjob.NewServiceWithSteam(db, igdb.NewClient("id", "secret", "http://localhost"), steam.NewClient("key"), nil)
	h := NewImportHandler(db, svc)

	w := serveImportRequest(t, h, user.ID, http.MethodPost, "/profile/steam/import", nil)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if !strings.Contains(w.Header().Get("Location"), "error=error.steam_not_linked") {
		t.Errorf("redirect = %q, want steam not linked error", w.Header().Get("Location"))
	}
}

func TestStartSteamImportSuccess(t *testing.T) {
	t.Setenv("TWITCH_CLIENT_ID", "id")
	t.Setenv("TWITCH_CLIENT_SECRET", "secret")

	db := testDB(t)
	defer db.Close()

	ctx := t.Context()
	user, err := models.CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if _, err := models.UpsertLinkedAccount(ctx, db, user.ID, "steam", "76561198012345678", "Gamer", "", "", nil); err != nil {
		t.Fatalf("UpsertLinkedAccount() error = %v", err)
	}

	svc := importjob.NewServiceWithSteam(db, igdb.NewClient("id", "secret", "http://localhost"), steam.NewClient("key"), nil)
	h := NewImportHandler(db, svc)

	w := serveImportRequest(t, h, user.ID, http.MethodPost, "/profile/steam/import", nil)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if loc := w.Header().Get("Location"); loc != "/profile" {
		t.Errorf("redirect = %q, want /profile", loc)
	}
}

func TestSteamImportStatusNoJob(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := t.Context()
	user, err := models.CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	svc := importjob.NewService(db, igdb.NewClient("id", "secret", "http://localhost"), nil)
	h := NewImportHandler(db, svc)

	router := newImportTestRouter(h)
	w := serveImportOnRouter(t, router, user.ID, http.MethodGet, "/profile/steam/import-status", nil)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestClearSteamLibrarySuccess(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := t.Context()
	user, err := models.CreateUser(ctx, db, uniqueUsername(t), "password123")
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
	if err := models.AddToLibrary(ctx, db, user.ID, game.ID, "Steam", nil); err != nil {
		t.Fatalf("AddToLibrary() error = %v", err)
	}

	svc := importjob.NewServiceWithSteam(db, igdb.NewClient("id", "secret", "http://localhost"), steam.NewClient("key"), nil)
	h := NewImportHandler(db, svc)

	w := serveImportRequest(t, h, user.ID, http.MethodPost, "/profile/steam/clear-library", nil)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if !strings.Contains(w.Header().Get("Location"), "steam_cleared=1") {
		t.Errorf("redirect = %q, want steam_cleared=1", w.Header().Get("Location"))
	}

	games, err := models.ListUserGames(ctx, db, user.ID)
	if err != nil {
		t.Fatalf("ListUserGames() error = %v", err)
	}
	if len(games) != 0 {
		t.Fatalf("library count = %d, want 0 after clear", len(games))
	}
}

func TestSteamImportStatusWithJob(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := t.Context()
	user, err := models.CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	job, err := models.CreateImportJob(ctx, db, user.ID, "steam")
	if err != nil {
		t.Fatalf("CreateImportJob() error = %v", err)
	}
	if err := models.SetImportJobTotal(ctx, db, job.ID, 10); err != nil {
		t.Fatalf("SetImportJobTotal() error = %v", err)
	}
	if err := models.UpdateImportJobProgress(ctx, db, job.ID, 4, 2, 2); err != nil {
		t.Fatalf("UpdateImportJobProgress() error = %v", err)
	}

	svc := importjob.NewService(db, igdb.NewClient("id", "secret", "http://localhost"), nil)
	h := NewImportHandler(db, svc)

	router := newImportTestRouter(h)
	w := serveImportOnRouter(t, router, user.ID, http.MethodGet, "/profile/steam/import-status", nil)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "Importing Steam library") {
		t.Errorf("body = %q, want import progress markup", w.Body.String())
	}
}

func TestStartXboxImportNotLinked(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := t.Context()
	user, err := models.CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	svc := importjob.NewServiceWithProviders(db, igdb.NewClient("id", "secret", "http://localhost"), nil, nil, xbox.NewClient("id", "secret"), nil)
	h := NewImportHandler(db, svc)

	w := serveImportRequest(t, h, user.ID, http.MethodPost, "/profile/xbox/import", nil)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if !strings.Contains(w.Header().Get("Location"), "error=error.xbox_not_linked") {
		t.Errorf("redirect = %q, want xbox not linked error", w.Header().Get("Location"))
	}
}

func TestStartXboxImportSuccess(t *testing.T) {
	t.Setenv("TWITCH_CLIENT_ID", "id")
	t.Setenv("TWITCH_CLIENT_SECRET", "secret")

	db := testDB(t)
	defer db.Close()

	ctx := t.Context()
	user, err := models.CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if _, err := models.UpsertLinkedAccount(ctx, db, user.ID, "xbox", "2535465432123456", "Gamer", "", "", nil); err != nil {
		t.Fatalf("UpsertLinkedAccount() error = %v", err)
	}

	svc := importjob.NewServiceWithProviders(db, igdb.NewClient("id", "secret", "http://localhost"), nil, nil, xbox.NewClient("id", "secret"), nil)
	h := NewImportHandler(db, svc)

	w := serveImportRequest(t, h, user.ID, http.MethodPost, "/profile/xbox/import", nil)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if loc := w.Header().Get("Location"); loc != "/profile" {
		t.Errorf("redirect = %q, want /profile", loc)
	}
}

func TestXboxImportStatusNoJob(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := t.Context()
	user, err := models.CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	svc := importjob.NewService(db, igdb.NewClient("id", "secret", "http://localhost"), nil)
	h := NewImportHandler(db, svc)

	router := newImportTestRouter(h)
	w := serveImportOnRouter(t, router, user.ID, http.MethodGet, "/profile/xbox/import-status", nil)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestClearXboxLibrarySuccess(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := t.Context()
	user, err := models.CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if _, err := models.UpsertLinkedAccount(ctx, db, user.ID, "xbox", "2535465432123456", "Gamer", "", "", nil); err != nil {
		t.Fatalf("UpsertLinkedAccount() error = %v", err)
	}

	cat, err := models.GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}
	game, err := models.FindOrCreateGameByXboxTitleID(ctx, db, 1144039928, "Halo Infinite", "", cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGameByXboxTitleID() error = %v", err)
	}
	if err := models.AddToLibrary(ctx, db, user.ID, game.ID, "Xbox", nil); err != nil {
		t.Fatalf("AddToLibrary() error = %v", err)
	}

	svc := importjob.NewServiceWithProviders(db, igdb.NewClient("id", "secret", "http://localhost"), nil, nil, xbox.NewClient("id", "secret"), nil)
	h := NewImportHandler(db, svc)

	w := serveImportRequest(t, h, user.ID, http.MethodPost, "/profile/xbox/clear-library", nil)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if !strings.Contains(w.Header().Get("Location"), "xbox_cleared=1") {
		t.Errorf("redirect = %q, want xbox_cleared=1", w.Header().Get("Location"))
	}

	games, err := models.ListUserGames(ctx, db, user.ID)
	if err != nil {
		t.Fatalf("ListUserGames() error = %v", err)
	}
	if len(games) != 0 {
		t.Fatalf("library count = %d, want 0 after clear", len(games))
	}
}

func TestXboxImportStatusWithJob(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := t.Context()
	user, err := models.CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	job, err := models.CreateImportJob(ctx, db, user.ID, "xbox")
	if err != nil {
		t.Fatalf("CreateImportJob() error = %v", err)
	}
	if err := models.SetImportJobTotal(ctx, db, job.ID, 10); err != nil {
		t.Fatalf("SetImportJobTotal() error = %v", err)
	}
	if err := models.UpdateImportJobProgress(ctx, db, job.ID, 4, 2, 2); err != nil {
		t.Fatalf("UpdateImportJobProgress() error = %v", err)
	}

	svc := importjob.NewService(db, igdb.NewClient("id", "secret", "http://localhost"), nil)
	h := NewImportHandler(db, svc)

	router := newImportTestRouter(h)
	w := serveImportOnRouter(t, router, user.ID, http.MethodGet, "/profile/xbox/import-status", nil)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "Importing Xbox library") {
		t.Errorf("body = %q, want import progress markup", w.Body.String())
	}
}

func TestCancelXboxImportStopsActiveJob(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := t.Context()
	user, err := models.CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if _, err := models.UpsertLinkedAccount(ctx, db, user.ID, "xbox", "2535465432123456", "Gamer", "", "", nil); err != nil {
		t.Fatalf("UpsertLinkedAccount() error = %v", err)
	}
	job, err := models.CreateImportJob(ctx, db, user.ID, "xbox")
	if err != nil {
		t.Fatalf("CreateImportJob() error = %v", err)
	}
	if err := models.SetImportJobTotal(ctx, db, job.ID, 10); err != nil {
		t.Fatalf("SetImportJobTotal() error = %v", err)
	}

	svc := importjob.NewServiceWithProviders(db, igdb.NewClient("id", "secret", "http://localhost"), nil, nil, xbox.NewClient("id", "secret"), nil)
	h := NewImportHandler(db, svc)

	w := serveImportRequest(t, h, user.ID, http.MethodPost, "/profile/xbox/import-cancel", nil)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if loc := w.Header().Get("Location"); loc != "/profile" {
		t.Errorf("redirect = %q, want /profile", loc)
	}

	got, err := models.GetImportJob(ctx, db, job.ID)
	if err != nil {
		t.Fatalf("GetImportJob() error = %v", err)
	}
	if got.Status != "failed" {
		t.Errorf("status = %q, want failed", got.Status)
	}
	active, err := models.HasActiveImportJob(ctx, db, user.ID, "xbox")
	if err != nil {
		t.Fatalf("HasActiveImportJob() error = %v", err)
	}
	if active {
		t.Fatal("expected no active job after cancel")
	}
}

func serveImportRequest(t *testing.T, h *ImportHandler, userID int64, method, path string, body *strings.Reader) *httptest.ResponseRecorder {
	t.Helper()
	router := newImportTestRouter(h)
	return serveImportOnRouter(t, router, userID, method, path, body)
}

func newImportTestRouter(h *ImportHandler) *gin.Engine {
	router := gin.New()
	store := cookie.NewStore([]byte("test-session-secret-32-chars!!"))
	router.Use(sessions.Sessions("session", store))
	router.SetHTMLTemplate(loadAllTemplates(filepath.Join("templates")))
	router.POST("/profile/steam/import", h.StartSteamImport)
	router.POST("/profile/steam/clear-library", h.ClearSteamLibrary)
	router.POST("/profile/steam/import-cancel", h.CancelSteamImport)
	router.GET("/profile/steam/import-status", h.SteamImportStatus)
	router.POST("/profile/xbox/import", h.StartXboxImport)
	router.POST("/profile/xbox/clear-library", h.ClearXboxLibrary)
	router.POST("/profile/xbox/import-cancel", h.CancelXboxImport)
	router.GET("/profile/xbox/import-status", h.XboxImportStatus)
	return router
}

func serveImportOnRouter(t *testing.T, router *gin.Engine, userID int64, method, path string, body *strings.Reader) *httptest.ResponseRecorder {
	t.Helper()

	setupRouter := gin.New()
	setupStore := cookie.NewStore([]byte("test-session-secret-32-chars!!"))
	setupRouter.Use(sessions.Sessions("session", setupStore))
	setupRouter.GET("/session/setup", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("user_id", userID)
		_ = session.Save()
		c.Status(http.StatusOK)
	})

	setupRecorder := httptest.NewRecorder()
	setupReq := httptest.NewRequest(http.MethodGet, "/session/setup", nil)
	setupRouter.ServeHTTP(setupRecorder, setupReq)

	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, body)
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	for _, cookie := range setupRecorder.Result().Cookies() {
		req.AddCookie(cookie)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}
