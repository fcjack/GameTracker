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
	"github.com/jacksoncoelho/game-tracker/internal/models"
)

func TestProfilePageWithEpicLinked(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	ctx := t.Context()
	user, _ := models.CreateUser(ctx, db, uniqueUsername(t), "password123")
	if _, err := models.UpsertLinkedAccount(ctx, db, user.ID, "epic", "9626f441055349ce8cb7d7d5a483eaa2", "EpicGamer", "", "", nil); err != nil {
		t.Fatalf("UpsertLinkedAccount() error = %v", err)
	}
	h := NewProfileHandler(db)
	router := gin.New()
	store := cookie.NewStore([]byte("test-session-secret-32-chars!!"))
	router.Use(sessions.Sessions("session", store))
	router.Use(LocaleMiddleware(db))
	router.SetHTMLTemplate(loadAllTemplates(filepath.Join("templates")))
	router.GET("/profile", h.ProfilePage)
	setupRouter := gin.New()
	setupStore := cookie.NewStore([]byte("test-session-secret-32-chars!!"))
	setupRouter.Use(sessions.Sessions("session", setupStore))
	setupRouter.GET("/session/setup", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("user_id", user.ID)
		session.Set("username", user.Username)
		session.Save()
		c.Status(http.StatusOK)
	})
	setupRecorder := httptest.NewRecorder()
	setupRouter.ServeHTTP(setupRecorder, httptest.NewRequest(http.MethodGet, "/session/setup", nil))
	req := httptest.NewRequest(http.MethodGet, "/profile", nil)
	for _, c := range setupRecorder.Result().Cookies() {
		req.AddCookie(c)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "clear_epic_library") && !strings.Contains(body, "Clear Epic") {
		t.Fatalf("body missing epic clear button: %s", body)
	}
	if !strings.Contains(body, "EpicGamer") {
		t.Fatalf("body missing epic display name: %s", body)
	}
}
