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
	"github.com/jacksoncoelho/game-tracker/internal/epic"
	"github.com/jacksoncoelho/game-tracker/internal/models"
)

type fakeEpicImporter struct {
	called bool
	userID int64
}

func (f *fakeEpicImporter) StartEpicImport(_ context.Context, userID int64) (*models.ImportJob, error) {
	f.called = true
	f.userID = userID
	return &models.ImportJob{UserID: userID, Provider: "epic"}, nil
}

func TestEpicInitiate(t *testing.T) {
	t.Parallel()

	h := &EpicHandler{
		client: epic.NewClient("client-id", "client-secret"),
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
	router.GET("/auth/epic", h.Initiate)

	setupRecorder := httptest.NewRecorder()
	setupReq := httptest.NewRequest(http.MethodGet, "/session/setup", nil)
	router.ServeHTTP(setupRecorder, setupReq)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/auth/epic", nil)
	for _, cookie := range setupRecorder.Result().Cookies() {
		req.AddCookie(cookie)
	}
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusFound {
		t.Fatalf("Initiate() status = %d, want %d", recorder.Code, http.StatusFound)
	}

	redirectURL := recorder.Header().Get("Location")
	for _, want := range []string{
		"www.epicgames.com/id/authorize",
		"client_id=client-id",
		"redirect_uri=http",
		"basic_profile",
	} {
		if !strings.Contains(redirectURL, want) {
			t.Errorf("redirect = %q, want substring %q", redirectURL, want)
		}
	}
}

func TestEpicInitiateNotConfigured(t *testing.T) {
	t.Parallel()

	h := &EpicHandler{
		client: epic.NewClient("", ""),
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/auth/epic", nil)

	h.Initiate(c)

	if !strings.Contains(w.Header().Get("Location"), "error=error.epic_not_configured") {
		t.Errorf("redirect = %q, want not configured error", w.Header().Get("Location"))
	}
}

func TestEpicCallbackSuccess(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	ctx := t.Context()
	user, err := models.CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	const (
		accountID   = "9626f441055349ce8cb7d7d5a483eaa2"
		displayName = "LinkedEpicGamer"
	)

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "epic-access-token",
			"refresh_token": "epic-refresh-token",
			"expires_in":    7200,
			"account_id":    accountID,
		})
	}))
	defer tokenServer.Close()

	userInfoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"sub":                accountID,
			"preferred_username": displayName,
		})
	}))
	defer userInfoServer.Close()

	encrypter, err := crypto.NewEncrypter("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewEncrypter() error = %v", err)
	}

	client := epic.NewClientWithHTTP("client-id", "client-secret", tokenServer.Client())
	client.SetEndpoints(tokenServer.URL, userInfoServer.URL)

	importer := &fakeEpicImporter{}
	h := &EpicHandler{
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
		session.Set(epicOAuthStateKey, "test-state")
		_ = session.Save()
		c.Status(http.StatusOK)
	})
	router.GET("/auth/epic/callback", h.Callback)

	setupRecorder := httptest.NewRecorder()
	setupReq := httptest.NewRequest(http.MethodGet, "/session/setup", nil)
	router.ServeHTTP(setupRecorder, setupReq)

	callbackRecorder := httptest.NewRecorder()
	callbackReq := httptest.NewRequest(http.MethodGet, "/auth/epic/callback?code=auth-code&state=test-state", nil)
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

	account, err := models.GetLinkedAccount(ctx, db, user.ID, "epic")
	if err != nil {
		t.Fatalf("GetLinkedAccount() error = %v", err)
	}
	if account.ExternalID != accountID {
		t.Errorf("external_id = %q, want %s", account.ExternalID, accountID)
	}
	if account.DisplayName != displayName {
		t.Errorf("display_name = %q, want %s", account.DisplayName, displayName)
	}
	if account.AccessTokenEnc == "" || account.RefreshTokenEnc == "" {
		t.Error("expected encrypted OAuth tokens to be stored")
	}
	if !importer.called {
		t.Fatal("expected StartEpicImport to be called after linking")
	}
	if importer.userID != user.ID {
		t.Errorf("StartEpicImport user_id = %d, want %d", importer.userID, user.ID)
	}
}

func TestEpicCallbackInvalidState(t *testing.T) {
	t.Parallel()

	encrypter, err := crypto.NewEncrypter("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewEncrypter() error = %v", err)
	}

	h := &EpicHandler{
		client:    epic.NewClient("client-id", "client-secret"),
		encrypter: encrypter,
	}

	w := serveEpicCallback(t, h, 1, "wrong-state", "/auth/epic/callback?code=auth-code&state=bad-state")
	if !strings.Contains(w.Header().Get("Location"), "error=error.epic_auth_failed") {
		t.Errorf("redirect = %q, want auth failed error", w.Header().Get("Location"))
	}
}

func TestEpicCallbackDenied(t *testing.T) {
	t.Parallel()

	encrypter, err := crypto.NewEncrypter("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewEncrypter() error = %v", err)
	}

	h := &EpicHandler{
		client:    epic.NewClient("client-id", "client-secret"),
		encrypter: encrypter,
	}

	w := serveEpicCallback(t, h, 1, "test-state", "/auth/epic/callback?error=access_denied&state=test-state")
	if !strings.Contains(w.Header().Get("Location"), "error=error.epic_auth_denied") {
		t.Errorf("redirect = %q, want auth denied error", w.Header().Get("Location"))
	}
}

func TestEpicCallbackRelinkUpserts(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	ctx := t.Context()
	user, err := models.CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	const (
		accountID   = "9626f441055349ce8cb7d7d5a483eaa2"
		displayName = "RelinkedEpicGamer"
	)

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "epic-access-token",
			"refresh_token": "epic-refresh-token",
			"expires_in":    7200,
			"account_id":    accountID,
		})
	}))
	defer tokenServer.Close()

	userInfoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"sub":                accountID,
			"preferred_username": displayName,
		})
	}))
	defer userInfoServer.Close()

	encrypter, err := crypto.NewEncrypter("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewEncrypter() error = %v", err)
	}

	client := epic.NewClientWithHTTP("client-id", "client-secret", tokenServer.Client())
	client.SetEndpoints(tokenServer.URL, userInfoServer.URL)

	h := &EpicHandler{
		db:        db,
		client:    client,
		encrypter: encrypter,
	}

	router := gin.New()
	store := cookie.NewStore([]byte("test-session-secret-32-chars!!"))
	router.Use(sessions.Sessions("session", store))
	router.GET("/session/setup", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("user_id", user.ID)
		session.Set(epicOAuthStateKey, "test-state")
		_ = session.Save()
		c.Status(http.StatusOK)
	})
	router.GET("/auth/epic/callback", h.Callback)

	runCallback := func() {
		setupRecorder := httptest.NewRecorder()
		setupReq := httptest.NewRequest(http.MethodGet, "/session/setup", nil)
		router.ServeHTTP(setupRecorder, setupReq)

		callbackRecorder := httptest.NewRecorder()
		callbackReq := httptest.NewRequest(http.MethodGet, "/auth/epic/callback?code=auth-code&state=test-state", nil)
		for _, cookie := range setupRecorder.Result().Cookies() {
			callbackReq.AddCookie(cookie)
		}
		router.ServeHTTP(callbackRecorder, callbackReq)
	}

	runCallback()
	first, err := models.GetLinkedAccount(ctx, db, user.ID, "epic")
	if err != nil {
		t.Fatalf("GetLinkedAccount() first link error = %v", err)
	}

	runCallback()
	second, err := models.GetLinkedAccount(ctx, db, user.ID, "epic")
	if err != nil {
		t.Fatalf("GetLinkedAccount() relink error = %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("relink id = %d, want same row %d", second.ID, first.ID)
	}
	if second.DisplayName != displayName {
		t.Errorf("display_name = %q, want %s", second.DisplayName, displayName)
	}
}

func serveEpicCallback(t *testing.T, h *EpicHandler, userID int64, state, path string) *httptest.ResponseRecorder {
	t.Helper()

	router := gin.New()
	store := cookie.NewStore([]byte("test-session-secret-32-chars!!"))
	router.Use(sessions.Sessions("session", store))
	router.GET("/session/setup", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("user_id", userID)
		session.Set(epicOAuthStateKey, state)
		_ = session.Save()
		c.Status(http.StatusOK)
	})
	router.GET("/auth/epic/callback", h.Callback)

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
