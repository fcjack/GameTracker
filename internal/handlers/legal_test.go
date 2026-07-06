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
	req.Header.Set("Accept-Language", "pt-BR,pt;q=0.9")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("PrivacyPage() status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if len(body) < 100 {
		t.Fatalf("PrivacyPage() body too short (%d bytes): %q", len(body), body)
	}
	if !strings.Contains(body, "Privacy Policy") {
		t.Fatalf("PrivacyPage() body missing English title (first 500 chars): %.500q", body)
	}
	if strings.Contains(body, "Política de Privacidade") {
		t.Fatalf("PrivacyPage() rendered Portuguese despite forced English locale")
	}
	if !strings.Contains(body, `lang="en"`) {
		t.Fatalf("PrivacyPage() html lang attribute is not English")
	}
}
