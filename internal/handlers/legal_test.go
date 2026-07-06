package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPrivacyPage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tmplDir := filepath.Join("templates")
	if _, err := os.Stat(tmplDir); err != nil {
		tmplDir = filepath.Join("..", "..", "templates")
	}

	r := gin.New()
	r.SetHTMLTemplate(loadAllTemplates(tmplDir))
	r.GET("/privacy", PrivacyPage)

	req := httptest.NewRequest(http.MethodGet, "/privacy", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("PrivacyPage() status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if len(body) < 100 {
		t.Fatalf("PrivacyPage() body too short (%d bytes): %q", len(body), body)
	}
	if !strings.Contains(body, "Privacy Policy") && !strings.Contains(body, "privacy.page_title") {
		t.Fatalf("PrivacyPage() body missing privacy policy content (first 500 chars): %.500q", body)
	}
}
