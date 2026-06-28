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
)

func TestXboxImportStatusRendersCancelLabel(t *testing.T) {
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

	svc := importjob.NewService(db, igdb.NewClient("id", "secret", "http://localhost"), nil)
	h := NewImportHandler(db, svc)

	router := gin.New()
	store := cookie.NewStore([]byte("test-session-secret-32-chars!!"))
	router.Use(sessions.Sessions("session", store))
	router.Use(LocaleMiddleware(db))
	router.SetHTMLTemplate(loadAllTemplates(filepath.Join("templates")))
	router.GET("/profile/xbox/import-status", h.XboxImportStatus)

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
	setupRouter.ServeHTTP(setupRecorder, httptest.NewRequest(http.MethodGet, "/session/setup", nil))

	req := httptest.NewRequest(http.MethodGet, "/profile/xbox/import-status", nil)
	for _, cookie := range setupRecorder.Result().Cookies() {
		req.AddCookie(cookie)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	body := recorder.Body.String()
	if strings.Contains(body, "import.cancel") {
		t.Fatalf("untranslated key in body:\n%s", body)
	}
	if !strings.Contains(body, "Cancel import") {
		t.Fatalf("missing cancel label in body:\n%s", body)
	}
	if strings.Contains(body, "import.cancel") || strings.Contains(body, "profile.cancel_import") {
		t.Fatalf("untranslated key in body:\n%s", body)
	}
}
