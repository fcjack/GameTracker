package importjob

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jacksoncoelho/game-tracker/internal/crypto"
	"github.com/jacksoncoelho/game-tracker/internal/igdb"
	"github.com/jacksoncoelho/game-tracker/internal/models"
	"github.com/jacksoncoelho/game-tracker/internal/xbox"
)

func TestStartXboxImportImportsGames(t *testing.T) {
	t.Setenv("TWITCH_CLIENT_ID", "test-client")
	t.Setenv("TWITCH_CLIENT_SECRET", "test-secret")

	const (
		accessToken = "microsoft-access-token"
		xuid        = "2535465432123456"
	)

	xboxServers := newMockXboxServers(t, xuid, []map[string]any{
		{
			"titleId":      "1144039928",
			"name":         "Halo Infinite",
			"displayImage": "https://images-eds-ssl.xboxlive.com/image?url=halo.jpg",
		},
		{
			"titleId":      "999888777",
			"name":         "Unknown Xbox Game",
			"displayImage": "https://images-eds-ssl.xboxlive.com/image?url=unknown.jpg",
		},
	})

	igdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
		case "/games":
			body, _ := io.ReadAll(r.Body)
			text := string(body)
			switch {
			case strings.Contains(text, `external_games.uid = "1144039928"`):
				json.NewEncoder(w).Encode([]map[string]any{{"id": 135590, "name": "Halo Infinite"}})
			case strings.Contains(text, "where id = 135590"):
				json.NewEncoder(w).Encode([]map[string]any{
					{
						"id":                 135590,
						"name":               "Halo Infinite",
						"category":           0,
						"first_release_date": time.Date(2021, 12, 8, 0, 0, 0, 0, time.UTC).Unix(),
						"platforms":          []map[string]any{{"name": "Xbox Series X|S"}},
					},
				})
			default:
				json.NewEncoder(w).Encode([]any{})
			}
		case "/external_games":
			json.NewEncoder(w).Encode([]any{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer igdbServer.Close()

	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	user, enc := setupXboxLinkedUser(t, db, xuid, accessToken)

	igdbClient := igdb.NewClient("test-client", "test-secret", igdbServer.URL)
	igdbClient.SetTokenURL(igdbServer.URL + "/token")
	igdbClient.SetHTTPClient(igdbServer.Client())

	xboxClient := xbox.NewClientWithHTTP("client-id", "secret", xboxServers.client())
	xboxClient.SetEndpoints("", xboxServers.userURL, xboxServers.xstsURL)
	xboxClient.SetTitleHubURL(xboxServers.titleHubURL)

	svc := NewServiceWithProviders(db, igdbClient, nil, nil, xboxClient, enc)

	job, err := svc.StartXboxImport(ctx, user.ID)
	if err != nil {
		t.Fatalf("StartXboxImport() error = %v", err)
	}

	waitForImportJob(t, db, job.ID, 5*time.Second)

	got, err := models.GetImportJob(ctx, db, job.ID)
	if err != nil {
		t.Fatalf("GetImportJob() error = %v", err)
	}
	if got.Status != "completed" {
		t.Fatalf("status = %q, want completed (error: %s)", got.Status, got.ErrorMessage)
	}
	if got.TotalCount != 2 {
		t.Errorf("total_count = %d, want 2", got.TotalCount)
	}
	if got.ImportedCount != 2 {
		t.Errorf("imported_count = %d, want 2", got.ImportedCount)
	}

	games, err := models.ListUserGames(ctx, db, user.ID)
	if err != nil {
		t.Fatalf("ListUserGames() error = %v", err)
	}
	if len(games) != 2 {
		t.Fatalf("library count = %d, want 2", len(games))
	}

	byName := map[string]*models.UserGameWithGame{}
	for _, g := range games {
		byName[g.Name] = g
	}
	if halo := byName["Halo Infinite"]; halo == nil || halo.Platform != "Xbox" || halo.IGDBId == nil {
		t.Errorf("Halo Infinite = %+v, want Xbox platform with IGDB id", halo)
	}
	if unknown := byName["Unknown Xbox Game"]; unknown == nil || unknown.Platform != "Xbox" {
		t.Errorf("Unknown Xbox Game = %+v, want Xbox platform", unknown)
	} else if unknown.IGDBId != nil {
		t.Errorf("Unknown Xbox Game igdb_id = %v, want nil", *unknown.IGDBId)
	}
}

func TestStartXboxImportSkipsAlreadyImported(t *testing.T) {
	t.Setenv("TWITCH_CLIENT_ID", "test-client")
	t.Setenv("TWITCH_CLIENT_SECRET", "test-secret")

	const (
		accessToken = "microsoft-access-token"
		xuid        = "2535465432123456"
	)

	xboxServers := newMockXboxServers(t, xuid, []map[string]any{
		{
			"titleId":      "1144039928",
			"name":         "Halo Infinite",
			"displayImage": "https://images-eds-ssl.xboxlive.com/image?url=halo.jpg",
		},
	})

	igdbCalls := 0
	igdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/games" || r.URL.Path == "/external_games" {
			igdbCalls++
		}
		switch r.URL.Path {
		case "/token":
			json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
		default:
			w.Write([]byte("[]"))
		}
	}))
	defer igdbServer.Close()

	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	user, enc := setupXboxLinkedUser(t, db, xuid, accessToken)

	cat, err := models.GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}
	existing, err := models.FindOrCreateGameByXboxTitleID(ctx, db, 1144039928, "Halo Infinite", "https://cdn.example.com/halo.jpg", cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGameByXboxTitleID() error = %v", err)
	}
	if err := models.AddToLibrary(ctx, db, user.ID, existing.ID, "Xbox", nil); err != nil {
		t.Fatalf("AddToLibrary() error = %v", err)
	}

	igdbClient := igdb.NewClient("test-client", "test-secret", igdbServer.URL)
	igdbClient.SetTokenURL(igdbServer.URL + "/token")
	igdbClient.SetHTTPClient(igdbServer.Client())

	xboxClient := xbox.NewClientWithHTTP("client-id", "secret", xboxServers.client())
	xboxClient.SetEndpoints("", xboxServers.userURL, xboxServers.xstsURL)
	xboxClient.SetTitleHubURL(xboxServers.titleHubURL)

	svc := NewServiceWithProviders(db, igdbClient, nil, nil, xboxClient, enc)

	job, err := svc.StartXboxImport(ctx, user.ID)
	if err != nil {
		t.Fatalf("StartXboxImport() error = %v", err)
	}
	waitForImportJob(t, db, job.ID, 5*time.Second)

	got, err := models.GetImportJob(ctx, db, job.ID)
	if err != nil {
		t.Fatalf("GetImportJob() error = %v", err)
	}
	if got.ImportedCount != 0 {
		t.Errorf("imported_count = %d, want 0", got.ImportedCount)
	}
	if got.SkippedCount != 1 {
		t.Errorf("skipped_count = %d, want 1", got.SkippedCount)
	}
	if igdbCalls != 0 {
		t.Errorf("igdb API calls = %d, want 0 for already-imported game", igdbCalls)
	}
}

func TestStartXboxImportSkipsSoftDeleted(t *testing.T) {
	t.Setenv("TWITCH_CLIENT_ID", "test-client")
	t.Setenv("TWITCH_CLIENT_SECRET", "test-secret")

	const (
		accessToken = "microsoft-access-token"
		xuid        = "2535465432123456"
	)

	xboxServers := newMockXboxServers(t, xuid, []map[string]any{
		{
			"titleId":      "1144039928",
			"name":         "Halo Infinite",
			"displayImage": "https://images-eds-ssl.xboxlive.com/image?url=halo.jpg",
		},
	})

	igdbCalls := 0
	igdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/games" || r.URL.Path == "/external_games" {
			igdbCalls++
		}
		switch r.URL.Path {
		case "/token":
			json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
		default:
			w.Write([]byte("[]"))
		}
	}))
	defer igdbServer.Close()

	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	user, enc := setupXboxLinkedUser(t, db, xuid, accessToken)

	cat, err := models.GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}
	existing, err := models.FindOrCreateGameByXboxTitleID(ctx, db, 1144039928, "Halo Infinite", "https://cdn.example.com/halo.jpg", cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGameByXboxTitleID() error = %v", err)
	}
	if err := models.AddToLibrary(ctx, db, user.ID, existing.ID, "Xbox", nil); err != nil {
		t.Fatalf("AddToLibrary() error = %v", err)
	}
	if err := models.RemoveFromLibrary(ctx, db, user.ID, existing.ID); err != nil {
		t.Fatalf("RemoveFromLibrary() error = %v", err)
	}

	igdbClient := igdb.NewClient("test-client", "test-secret", igdbServer.URL)
	igdbClient.SetTokenURL(igdbServer.URL + "/token")
	igdbClient.SetHTTPClient(igdbServer.Client())

	xboxClient := xbox.NewClientWithHTTP("client-id", "secret", xboxServers.client())
	xboxClient.SetEndpoints("", xboxServers.userURL, xboxServers.xstsURL)
	xboxClient.SetTitleHubURL(xboxServers.titleHubURL)

	svc := NewServiceWithProviders(db, igdbClient, nil, nil, xboxClient, enc)

	job, err := svc.StartXboxImport(ctx, user.ID)
	if err != nil {
		t.Fatalf("StartXboxImport() error = %v", err)
	}
	waitForImportJob(t, db, job.ID, 5*time.Second)

	got, err := models.GetImportJob(ctx, db, job.ID)
	if err != nil {
		t.Fatalf("GetImportJob() error = %v", err)
	}
	if got.ImportedCount != 0 {
		t.Errorf("imported_count = %d, want 0", got.ImportedCount)
	}
	if got.SkippedCount != 1 {
		t.Errorf("skipped_count = %d, want 1", got.SkippedCount)
	}
	if igdbCalls != 0 {
		t.Errorf("igdb API calls = %d, want 0 for soft-deleted game", igdbCalls)
	}

	games, err := models.ListUserGames(ctx, db, user.ID)
	if err != nil {
		t.Fatalf("ListUserGames() error = %v", err)
	}
	if len(games) != 0 {
		t.Fatalf("library count = %d, want 0 after soft-deleted Xbox sync", len(games))
	}
}

func TestStartXboxImportReturnsExistingActiveJob(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	user, enc := setupXboxLinkedUser(t, db, "2535465432123456", "access-token")

	existing, err := models.CreateImportJob(ctx, db, user.ID, "xbox")
	if err != nil {
		t.Fatalf("CreateImportJob() error = %v", err)
	}

	svc := NewServiceWithProviders(db, igdb.NewClient("id", "secret", "http://localhost"), nil, nil, xbox.NewClient("id", "secret"), enc)
	job, err := svc.StartXboxImport(ctx, user.ID)
	if err != nil {
		t.Fatalf("StartXboxImport() error = %v", err)
	}
	if job.ID != existing.ID {
		t.Errorf("job id = %d, want existing %d", job.ID, existing.ID)
	}
}

func TestStartXboxImportRestartsStalePendingJob(t *testing.T) {
	t.Setenv("TWITCH_CLIENT_ID", "test-client")
	t.Setenv("TWITCH_CLIENT_SECRET", "test-secret")

	const xuid = "2535465432123456"

	xboxServers := newMockXboxServers(t, xuid, []map[string]any{})

	igdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
		default:
			w.Write([]byte("[]"))
		}
	}))
	defer igdbServer.Close()

	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	user, enc := setupXboxLinkedUser(t, db, xuid, "access-token")

	stale, err := models.CreateImportJob(ctx, db, user.ID, "xbox")
	if err != nil {
		t.Fatalf("CreateImportJob() error = %v", err)
	}

	igdbClient := igdb.NewClient("test-client", "test-secret", igdbServer.URL)
	igdbClient.SetTokenURL(igdbServer.URL + "/token")
	igdbClient.SetHTTPClient(igdbServer.Client())

	xboxClient := xbox.NewClientWithHTTP("client-id", "secret", xboxServers.client())
	xboxClient.SetEndpoints("", xboxServers.userURL, xboxServers.xstsURL)
	xboxClient.SetTitleHubURL(xboxServers.titleHubURL)

	svc := NewServiceWithProviders(db, igdbClient, nil, nil, xboxClient, enc)

	job, err := svc.StartXboxImport(ctx, user.ID)
	if err != nil {
		t.Fatalf("StartXboxImport() error = %v", err)
	}
	if job.ID == stale.ID {
		t.Fatalf("expected new job after restart, got stale id %d", stale.ID)
	}

	failed, err := models.GetImportJob(ctx, db, stale.ID)
	if err != nil {
		t.Fatalf("GetImportJob(stale) error = %v", err)
	}
	if failed.Status != "failed" {
		t.Errorf("stale status = %q, want failed", failed.Status)
	}
}

func TestRunXboxImportWithoutIGDBCredentials(t *testing.T) {
	t.Setenv("TWITCH_CLIENT_ID", "")
	t.Setenv("TWITCH_CLIENT_SECRET", "")

	const xuid = "2535465432123456"

	xboxServers := newMockXboxServers(t, xuid, []map[string]any{
		{
			"titleId":      "555666777",
			"name":         "Xbox Only Game",
			"displayImage": "https://images-eds-ssl.xboxlive.com/image?url=only.jpg",
		},
	})

	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	user, enc := setupXboxLinkedUser(t, db, xuid, "access-token")

	xboxClient := xbox.NewClientWithHTTP("client-id", "secret", xboxServers.client())
	xboxClient.SetEndpoints("", xboxServers.userURL, xboxServers.xstsURL)
	xboxClient.SetTitleHubURL(xboxServers.titleHubURL)

	svc := NewServiceWithProviders(
		db,
		igdb.NewClient("", "", "http://localhost"),
		nil,
		nil,
		xboxClient,
		enc,
	)

	job, err := svc.StartXboxImport(ctx, user.ID)
	if err != nil {
		t.Fatalf("StartXboxImport() error = %v", err)
	}

	waitForImportJob(t, db, job.ID, 5*time.Second)

	got, err := models.GetImportJob(ctx, db, job.ID)
	if err != nil {
		t.Fatalf("GetImportJob() error = %v", err)
	}
	if got.Status != "completed" {
		t.Fatalf("status = %q, want completed (error: %s)", got.Status, got.ErrorMessage)
	}
	if got.ImportedCount != 1 {
		t.Errorf("imported_count = %d, want 1", got.ImportedCount)
	}
}

func TestStartXboxImportRecordsAPIFailure(t *testing.T) {
	t.Setenv("TWITCH_CLIENT_ID", "")
	t.Setenv("TWITCH_CLIENT_SECRET", "")

	const xuid = "2535465432123456"

	titleHubServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"statusCode":403,"message":"Forbidden"}`))
	}))
	defer titleHubServer.Close()

	userServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"Token": "user-token"})
	}))
	defer userServer.Close()

	xstsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"Token": "xsts-token",
			"DisplayClaims": map[string]any{
				"xui": []map[string]string{{
					"uhs": "user-hash",
					"xid": xuid,
				}},
			},
		})
	}))
	defer xstsServer.Close()

	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	user, enc := setupXboxLinkedUser(t, db, xuid, "access-token")

	xboxClient := xbox.NewClientWithHTTP("client-id", "secret", userServer.Client())
	xboxClient.SetEndpoints("", userServer.URL, xstsServer.URL)
	xboxClient.SetTitleHubURL(titleHubServer.URL)

	svc := NewServiceWithProviders(db, igdb.NewClient("", "", "http://localhost"), nil, nil, xboxClient, enc)

	job, err := svc.StartXboxImport(ctx, user.ID)
	if err != nil {
		t.Fatalf("StartXboxImport() error = %v", err)
	}

	waitForImportJob(t, db, job.ID, 5*time.Second)

	got, err := models.GetImportJob(ctx, db, job.ID)
	if err != nil {
		t.Fatalf("GetImportJob() error = %v", err)
	}
	if got.Status != "failed" {
		t.Fatalf("status = %q, want failed", got.Status)
	}
	if got.ErrorMessage == "" || !strings.Contains(got.ErrorMessage, "403") {
		t.Errorf("error_message = %q, want API failure details", got.ErrorMessage)
	}
}

type mockXboxServers struct {
	userURL      string
	xstsURL      string
	titleHubURL  string
	httpClient   *http.Client
}

func (m *mockXboxServers) client() *http.Client {
	return m.httpClient
}

func newMockXboxServers(t *testing.T, xuid string, titles []map[string]any) *mockXboxServers {
	t.Helper()

	userServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"Token": "user-token"})
	}))

	xstsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"Token": "xsts-token",
			"DisplayClaims": map[string]any{
				"xui": []map[string]string{{
					"uhs": "user-hash",
					"xid": xuid,
					"gtg": "TestGamer",
				}},
			},
		})
	}))

	titleHubServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"titles": titles})
	}))

	t.Cleanup(userServer.Close)
	t.Cleanup(xstsServer.Close)
	t.Cleanup(titleHubServer.Close)

	return &mockXboxServers{
		userURL:     userServer.URL,
		xstsURL:     xstsServer.URL,
		titleHubURL: titleHubServer.URL,
		httpClient:  userServer.Client(),
	}
}

func setupXboxLinkedUser(t *testing.T, db *pgxpool.Pool, xuid, accessToken string) (*models.User, *crypto.Encrypter) {
	t.Helper()

	ctx := context.Background()
	enc, err := crypto.NewEncrypter("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewEncrypter() error = %v", err)
	}

	user, err := models.CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	expiresAt := time.Now().Add(2 * time.Hour)
	accessEnc, err := enc.Encrypt(accessToken)
	if err != nil {
		t.Fatalf("Encrypt(access) error = %v", err)
	}
	refreshEnc, err := enc.Encrypt("refresh-token")
	if err != nil {
		t.Fatalf("Encrypt(refresh) error = %v", err)
	}

	if _, err := models.UpsertLinkedAccount(
		ctx, db, user.ID, "xbox", xuid, "TestGamer",
		accessEnc, refreshEnc, &expiresAt,
	); err != nil {
		t.Fatalf("UpsertLinkedAccount() error = %v", err)
	}

	return user, enc
}
