package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jacksoncoelho/game-tracker/internal/database"
	"github.com/jacksoncoelho/game-tracker/internal/models"
)

func TestExtractSteamID(t *testing.T) {
	tests := []struct {
		name      string
		claimedID string
		want      string
	}{
		{
			name:      "standard steam openid url",
			claimedID: "https://steamcommunity.com/openid/id/76561198012345678",
			want:      "76561198012345678",
		},
		{
			name:      "trailing slash",
			claimedID: "https://steamcommunity.com/openid/id/76561198012345678/",
			want:      "",
		},
		{
			name:      "no id segment",
			claimedID: "https://steamcommunity.com/openid/id/",
			want:      "",
		},
		{
			name:      "empty",
			claimedID: "",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractSteamID(tt.claimedID); got != tt.want {
				t.Errorf("extractSteamID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSteamInitiate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	openIDServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer openIDServer.Close()

	h := &SteamHandler{
		openIDURL: openIDServer.URL,
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "http://localhost:8080/auth/steam", nil)

	h.Initiate(c)

	if w.Code != http.StatusFound {
		t.Fatalf("Initiate() status = %d, want %d", w.Code, http.StatusFound)
	}

	redirectURL, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse redirect URL: %v", err)
	}

	if redirectURL.Host != "steamcommunity.com" {
		t.Errorf("redirect host = %q, want steamcommunity.com", redirectURL.Host)
	}

	q := redirectURL.Query()
	checks := map[string]string{
		"openid.ns":         "http://specs.openid.net/auth/2.0",
		"openid.mode":       "checkid_setup",
		"openid.return_to":  "http://localhost:8080/auth/steam/callback",
		"openid.realm":      "http://localhost:8080",
		"openid.identity":   "http://specs.openid.net/auth/2.0/identifier_select",
		"openid.claimed_id": "http://specs.openid.net/auth/2.0/identifier_select",
	}
	for key, want := range checks {
		if got := q.Get(key); got != want {
			t.Errorf("redirect param %s = %q, want %q", key, got, want)
		}
	}
}

func TestSteamInitiateHTTPS(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := &SteamHandler{}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "https://example.com/auth/steam", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	c.Request = req

	h.Initiate(c)

	redirectURL, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse redirect URL: %v", err)
	}

	if got := redirectURL.Query().Get("openid.return_to"); got != "https://example.com/auth/steam/callback" {
		t.Errorf("return_to = %q, want https callback URL", got)
	}
}

func TestFetchSteamPersonaName(t *testing.T) {
	const steamID = "76561198012345678"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/ISteamUser/GetPlayerSummaries/v0002/") {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("steamids"); got != steamID {
			t.Errorf("steamids = %q, want %s", got, steamID)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"response": map[string]any{
				"players": []map[string]string{
					{"personaname": "TestGamer"},
				},
			},
		})
	}))
	defer server.Close()

	h := &SteamHandler{
		steamAPIURL: server.URL,
		httpClient:  server.Client(),
	}

	name, err := h.fetchSteamPersonaName(steamID, "fake-api-key")
	if err != nil {
		t.Fatalf("fetchSteamPersonaName() error = %v", err)
	}
	if name != "TestGamer" {
		t.Errorf("persona name = %q, want TestGamer", name)
	}
}

func TestSteamCallbackVerificationFailure(t *testing.T) {
	openIDServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("is_valid:false\n"))
	}))
	defer openIDServer.Close()

	h := &SteamHandler{
		openIDURL:  openIDServer.URL,
		httpClient: openIDServer.Client(),
	}

	w := serveSteamCallback(t, h, 1, "/auth/steam/callback?openid.claimed_id=https://steamcommunity.com/openid/id/76561198012345678")

	if w.Code != http.StatusFound {
		t.Fatalf("Callback() status = %d, want %d", w.Code, http.StatusFound)
	}
	if !strings.Contains(w.Header().Get("Location"), "error=error.steam_verification_failed") {
		t.Errorf("redirect = %q, want verification failed error", w.Header().Get("Location"))
	}
}

func TestSteamCallbackInvalidClaimedID(t *testing.T) {
	openIDServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("is_valid:true\n"))
	}))
	defer openIDServer.Close()

	h := &SteamHandler{
		openIDURL:  openIDServer.URL,
		httpClient: openIDServer.Client(),
	}

	w := serveSteamCallback(t, h, 1, "/auth/steam/callback")

	if !strings.Contains(w.Header().Get("Location"), "error=error.invalid_steam_response") {
		t.Errorf("redirect = %q, want invalid response error", w.Header().Get("Location"))
	}
}

func TestSteamCallbackInvalidSteamID(t *testing.T) {
	openIDServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("is_valid:true\n"))
	}))
	defer openIDServer.Close()

	h := &SteamHandler{
		openIDURL:  openIDServer.URL,
		httpClient: openIDServer.Client(),
	}

	w := serveSteamCallback(t, h, 1, "/auth/steam/callback?openid.claimed_id=https://steamcommunity.com/openid/id/not-a-number")

	if !strings.Contains(w.Header().Get("Location"), "error=error.invalid_steam_id") {
		t.Errorf("redirect = %q, want invalid steam id error", w.Header().Get("Location"))
	}
}

func TestSteamCallbackSuccess(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	ctx := t.Context()
	user, err := models.CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	const steamID = "76561198012345678"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "GetPlayerSummaries") {
			json.NewEncoder(w).Encode(map[string]any{
				"response": map[string]any{
					"players": []map[string]string{
						{"personaname": "LinkedGamer"},
					},
				},
			})
			return
		}
		w.Write([]byte("is_valid:true\n"))
	}))
	defer server.Close()

	t.Setenv("STEAM_API_KEY", "test-key")

	h := &SteamHandler{
		db:          db,
		openIDURL:   server.URL,
		steamAPIURL: server.URL,
		httpClient:  server.Client(),
	}

	claimedID := "https://steamcommunity.com/openid/id/" + steamID
	w := serveSteamCallback(t, h, user.ID, "/auth/steam/callback?openid.claimed_id="+url.QueryEscape(claimedID))

	if w.Code != http.StatusFound {
		t.Fatalf("Callback() status = %d, want %d", w.Code, http.StatusFound)
	}
	if got := w.Header().Get("Location"); got != "/profile" {
		t.Errorf("redirect = %q, want /profile", got)
	}

	account, err := models.GetLinkedAccount(ctx, db, user.ID, "steam")
	if err != nil {
		t.Fatalf("GetLinkedAccount() error = %v", err)
	}
	if account.ExternalID != steamID {
		t.Errorf("external_id = %q, want %s", account.ExternalID, steamID)
	}
	if account.DisplayName != "LinkedGamer" {
		t.Errorf("display_name = %q, want LinkedGamer", account.DisplayName)
	}
}

func serveSteamCallback(t *testing.T, h *SteamHandler, userID int64, path string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	store := cookie.NewStore([]byte("test-session-secret-32-chars!!"))
	router.Use(sessions.Sessions("session", store))
	router.GET("/session/setup", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("user_id", userID)
		_ = session.Save()
		c.Status(http.StatusOK)
	})
	router.GET("/auth/steam/callback", h.Callback)

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

func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	chdirToModuleRoot(t)

	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		dbURL = "postgres://gametracker:gametracker@localhost:5432/gametracker?sslmode=disable"
	}

	db, err := database.Connect(dbURL)
	if err != nil {
		t.Skipf("database not available: %v", err)
	}
	if err := database.RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations() error = %v", err)
	}
	return db
}

func chdirToModuleRoot(t *testing.T) {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if err := os.Chdir(dir); err != nil {
				t.Fatal(err)
			}
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find module root")
		}
		dir = parent
	}
}

func uniqueUsername(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("testuser_%d", time.Now().UnixNano())
}
