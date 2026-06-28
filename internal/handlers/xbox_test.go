package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/jacksoncoelho/game-tracker/internal/crypto"
	"github.com/jacksoncoelho/game-tracker/internal/models"
	"github.com/jacksoncoelho/game-tracker/internal/xbox"
)

type fakeXboxImporter struct {
	called bool
	userID int64
}

func (f *fakeXboxImporter) StartXboxImport(_ context.Context, userID int64) (*models.ImportJob, error) {
	f.called = true
	f.userID = userID
	return &models.ImportJob{UserID: userID, Provider: "xbox"}, nil
}

func TestXboxInitiate(t *testing.T) {
	t.Parallel()

	h := &XboxHandler{
		client: xbox.NewClient("client-id", "client-secret"),
	}

	router := gin.New()
	store := cookie.NewStore([]byte("test-session-secret-32-chars!!"))
	router.Use(sessions.Sessions("session", store))
	router.GET("/session/setup", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("user_id", int64(1))
		_ = session.Save()
		c.Status(http.StatusOK)
	})
	router.GET("/auth/xbox", h.Initiate)

	setupRecorder := httptest.NewRecorder()
	setupReq := httptest.NewRequest(http.MethodGet, "/session/setup", nil)
	router.ServeHTTP(setupRecorder, setupReq)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/auth/xbox", nil)
	for _, cookie := range setupRecorder.Result().Cookies() {
		req.AddCookie(cookie)
	}
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusFound {
		t.Fatalf("Initiate() status = %d, want %d", recorder.Code, http.StatusFound)
	}

	redirectURL := recorder.Header().Get("Location")
	for _, want := range []string{
		"login.microsoftonline.com",
		"client_id=client-id",
		"redirect_uri=http",
		"xboxlive.signin",
	} {
		if !strings.Contains(redirectURL, want) {
			t.Errorf("redirect = %q, want substring %q", redirectURL, want)
		}
	}
}

func TestXboxInitiateNotConfigured(t *testing.T) {
	t.Parallel()

	h := &XboxHandler{
		client: xbox.NewClient("", ""),
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/auth/xbox", nil)

	h.Initiate(c)

	if !strings.Contains(w.Header().Get("Location"), "error=error.xbox_not_configured") {
		t.Errorf("redirect = %q, want not configured error", w.Header().Get("Location"))
	}
}

func TestXboxCallbackSuccess(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	ctx := t.Context()
	user, err := models.CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	const (
		xuid     = "2535465432123456"
		gamertag = "LinkedGamer"
	)

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "microsoft-access-token",
			"refresh_token": "microsoft-refresh-token",
			"expires_in":    3600,
		})
	}))
	defer tokenServer.Close()

	userServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"Token": "xbox-user-token"})
	}))
	defer userServer.Close()

	xstsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"Token": "xsts-token",
			"DisplayClaims": map[string]any{
				"xui": []map[string]string{{
					"uhs": "user-hash",
					"xid": xuid,
					"gtg": gamertag,
				}},
			},
		})
	}))
	defer xstsServer.Close()

	encrypter, err := crypto.NewEncrypter("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewEncrypter() error = %v", err)
	}

	client := xbox.NewClientWithHTTP("client-id", "client-secret", tokenServer.Client())
	client.SetEndpoints(tokenServer.URL, userServer.URL, xstsServer.URL)

	importer := &fakeXboxImporter{}
	h := &XboxHandler{
		db:            db,
		client:        client,
		encrypter:     encrypter,
		importService: importer,
	}

	router := gin.New()
	store := cookie.NewStore([]byte("test-session-secret-32-chars!!"))
	router.Use(sessions.Sessions("session", store))
	router.GET("/session/setup", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("user_id", user.ID)
		session.Set(xboxOAuthStateKey, "test-state")
		_ = session.Save()
		c.Status(http.StatusOK)
	})
	router.GET("/auth/xbox/callback", h.Callback)

	setupRecorder := httptest.NewRecorder()
	setupReq := httptest.NewRequest(http.MethodGet, "/session/setup", nil)
	router.ServeHTTP(setupRecorder, setupReq)

	callbackRecorder := httptest.NewRecorder()
	callbackReq := httptest.NewRequest(http.MethodGet, "/auth/xbox/callback?code=auth-code&state=test-state", nil)
	for _, cookie := range setupRecorder.Result().Cookies() {
		callbackReq.AddCookie(cookie)
	}
	router.ServeHTTP(callbackRecorder, callbackReq)

	if callbackRecorder.Code != http.StatusFound {
		t.Fatalf("Callback() status = %d, want %d", callbackRecorder.Code, http.StatusFound)
	}
	if got := callbackRecorder.Header().Get("Location"); got != "/profile" {
		t.Errorf("redirect = %q, want /profile", got)
	}

	account, err := models.GetLinkedAccount(ctx, db, user.ID, "xbox")
	if err != nil {
		t.Fatalf("GetLinkedAccount() error = %v", err)
	}
	if account.ExternalID != xuid {
		t.Errorf("external_id = %q, want %s", account.ExternalID, xuid)
	}
	if account.DisplayName != gamertag {
		t.Errorf("display_name = %q, want %s", account.DisplayName, gamertag)
	}
	if account.AccessTokenEnc == "" || account.RefreshTokenEnc == "" {
		t.Error("expected encrypted OAuth tokens to be stored")
	}
	if !importer.called {
		t.Fatal("expected StartXboxImport to be called after linking")
	}
	if importer.userID != user.ID {
		t.Errorf("StartXboxImport user_id = %d, want %d", importer.userID, user.ID)
	}
}

func TestXboxCallbackInvalidState(t *testing.T) {
	t.Parallel()

	encrypter, err := crypto.NewEncrypter("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewEncrypter() error = %v", err)
	}

	h := &XboxHandler{
		client:    xbox.NewClient("client-id", "client-secret"),
		encrypter: encrypter,
	}

	w := serveXboxCallback(t, h, 1, "wrong-state", "/auth/xbox/callback?code=auth-code&state=bad-state")
	if !strings.Contains(w.Header().Get("Location"), "error=error.xbox_auth_failed") {
		t.Errorf("redirect = %q, want auth failed error", w.Header().Get("Location"))
	}
}

func serveXboxCallback(t *testing.T, h *XboxHandler, userID int64, state, path string) *httptest.ResponseRecorder {
	t.Helper()

	router := gin.New()
	store := cookie.NewStore([]byte("test-session-secret-32-chars!!"))
	router.Use(sessions.Sessions("session", store))
	router.GET("/session/setup", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("user_id", userID)
		session.Set(xboxOAuthStateKey, state)
		_ = session.Save()
		c.Status(http.StatusOK)
	})
	router.GET("/auth/xbox/callback", h.Callback)

	setupRecorder := httptest.NewRecorder()
	setupReq := httptest.NewRequest(http.MethodGet, "/session/setup", nil)
	router.ServeHTTP(setupRecorder, setupReq)

	callbackRecorder := httptest.NewRecorder()
	callbackReq := httptest.NewRequest(http.MethodGet, path, nil)
	for _, cookie := range setupRecorder.Result().Cookies() {
		callbackReq.AddCookie(cookie)
	}
	router.ServeHTTP(callbackRecorder, callbackReq)
	return callbackRecorder
}
