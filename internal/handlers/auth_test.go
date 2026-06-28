package handlers

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

func authTestRouter(h *AuthHandler) *gin.Engine {
	r := gin.New()
	store := cookie.NewStore([]byte("test-session-secret-32-chars!!"))
	r.Use(sessions.Sessions("session", store))
	tmpl := template.Must(template.New("").Parse(`
{{define "auth/login"}}{{.error}}{{end}}
{{define "auth/register"}}{{.error}}{{end}}
`))
	r.SetHTMLTemplate(tmpl)
	r.POST("/login", h.Login)
	r.POST("/register", h.Register)
	r.GET("/register", h.RegisterPage)
	return r
}

func TestRegisterDisabled(t *testing.T) {
	t.Parallel()

	h := &AuthHandler{registrationEnabled: false}
	r := authTestRouter(h)

	form := url.Values{
		"username":         {"alice"},
		"password":         {"secret"},
		"confirm_password": {"secret"},
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("Register() status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestRegisterPageRedirectsWhenDisabled(t *testing.T) {
	t.Parallel()

	h := &AuthHandler{registrationEnabled: false}
	r := authTestRouter(h)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/register", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("RegisterPage() status = %d, want %d", w.Code, http.StatusFound)
	}
	if location := w.Header().Get("Location"); location != "/login" {
		t.Fatalf("RegisterPage() Location = %q, want /login", location)
	}
}
