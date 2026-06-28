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

func TestProfilePageRenders(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := t.Context()
	user, err := models.CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
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
		_ = session.Save()
		c.Status(http.StatusOK)
	})

	setupRecorder := httptest.NewRecorder()
	setupReq := httptest.NewRequest(http.MethodGet, "/session/setup", nil)
	setupRouter.ServeHTTP(setupRecorder, setupReq)

	req := httptest.NewRequest(http.MethodGet, "/profile", nil)
	for _, cookie := range setupRecorder.Result().Cookies() {
		req.AddCookie(cookie)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	body := recorder.Body.String()
	if len(body) < 100 {
		t.Fatalf("body too short (%d bytes): %q", len(body), body)
	}
	if !strings.Contains(body, "profile.title") && !strings.Contains(body, "Profile") {
		t.Fatalf("body missing profile content:\n%s", body)
	}
	if !strings.Contains(body, "</html>") {
		t.Fatalf("body missing closing html tag (truncated template):\n%s", body)
	}
}
