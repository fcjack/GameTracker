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
	"github.com/jacksoncoelho/game-tracker/internal/epic"
	"github.com/jacksoncoelho/game-tracker/internal/igdb"
	"github.com/jacksoncoelho/game-tracker/internal/models"
)

func newEpicImportService(db *pgxpool.Pool, igdbClient *igdb.Client, epicClient *epic.Client, enc *crypto.Encrypter) *Service {
	svc := NewServiceWithProviders(db, igdbClient, nil, nil, nil, enc)
	svc.SetEpicClient(epicClient)
	return svc
}

func TestStartEpicImportImportsGames(t *testing.T) {
	t.Setenv("TWITCH_CLIENT_ID", "test-client")
	t.Setenv("TWITCH_CLIENT_SECRET", "test-secret")

	const (
		accessToken = "epic-access-token"
		hadesID     = "11111111-1111-1111-1111-111111111111"
		unknownID   = "22222222-2222-2222-2222-222222222222"
		accountID   = "9626f441055349ce8cb7d7d5a483eaa2"
	)

	libraryServer := newMockEpicLibraryServer(t, accessToken, []map[string]any{
		{
			"appName":       "Hades",
			"namespace":     "a1234567890abcdef1234567890abcdef",
			"catalogItemId": hadesID,
			"sandboxType":   "PUBLICGAME",
			"metadata": map[string]any{
				"title": "Hades",
				"keyImages": []map[string]string{
					{"type": "DieselGameBoxTall", "url": "https://cdn.example/hades.jpg"},
				},
			},
		},
		{
			"appName":       "Unknown Epic Game",
			"namespace":     "a1234567890abcdef1234567890abcdef",
			"catalogItemId": unknownID,
			"sandboxType":   "PUBLICGAME",
			"metadata":      map[string]any{"title": "Unknown Epic Game"},
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
			case strings.Contains(text, `external_games.uid = "`+hadesID+`"`):
				json.NewEncoder(w).Encode([]map[string]any{{"id": 119133, "name": "Hades"}})
			case strings.Contains(text, "where id = 119133"):
				json.NewEncoder(w).Encode([]map[string]any{
					{
						"id":                 119133,
						"name":               "Hades",
						"category":           0,
						"first_release_date": time.Date(2020, 9, 17, 0, 0, 0, 0, time.UTC).Unix(),
						"platforms":          []map[string]any{{"name": "PC (Microsoft Windows)"}},
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
	user, enc := setupEpicLinkedUser(t, db, accountID, accessToken)

	igdbClient := igdb.NewClient("test-client", "test-secret", igdbServer.URL)
	igdbClient.SetTokenURL(igdbServer.URL + "/token")
	igdbClient.SetHTTPClient(igdbServer.Client())

	epicClient := epic.NewClientWithHTTP("client-id", "secret", libraryServer.Client())
	epicClient.SetLibraryURL(libraryServer.URL)

	svc := newEpicImportService(db, igdbClient, epicClient, enc)

	job, err := svc.StartEpicImport(ctx, user.ID)
	if err != nil {
		t.Fatalf("StartEpicImport() error = %v", err)
	}

	waitForImportJob(t, db, job.ID, 5*time.Second)

	got, err := models.GetImportJob(ctx, db, job.ID)
	if err != nil {
		t.Fatalf("GetImportJob() error = %v", err)
	}
	if got.Status != "completed" {
		t.Fatalf("status = %q, want completed (error: %s)", got.Status, got.ErrorMessage)
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
}

func TestStartEpicImportSkipsAlreadyImported(t *testing.T) {
	t.Setenv("TWITCH_CLIENT_ID", "test-client")
	t.Setenv("TWITCH_CLIENT_SECRET", "test-secret")

	const (
		accessToken = "epic-access-token"
		catalogID   = "33333333-3333-3333-3333-333333333333"
		accountID   = "9626f441055349ce8cb7d7d5a483eaa2"
	)

	libraryServer := newMockEpicLibraryServer(t, accessToken, []map[string]any{
		{
			"appName":       "Already Imported",
			"namespace":     "ns",
			"catalogItemId": catalogID,
			"sandboxType":   "PUBLICGAME",
			"metadata":      map[string]any{"title": "Already Imported"},
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
	user, enc := setupEpicLinkedUser(t, db, accountID, accessToken)

	cat, err := models.GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}
	existing, err := models.FindOrCreateGameByEpicCatalogItemID(ctx, db, catalogID, "Already Imported", "https://cdn.example/existing.jpg", cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGameByEpicCatalogItemID() error = %v", err)
	}
	if err := models.AddToLibrary(ctx, db, user.ID, existing.ID, "Epic", nil); err != nil {
		t.Fatalf("AddToLibrary() error = %v", err)
	}

	igdbClient := igdb.NewClient("test-client", "test-secret", igdbServer.URL)
	igdbClient.SetTokenURL(igdbServer.URL + "/token")
	igdbClient.SetHTTPClient(igdbServer.Client())

	epicClient := epic.NewClientWithHTTP("client-id", "secret", libraryServer.Client())
	epicClient.SetLibraryURL(libraryServer.URL)

	svc := newEpicImportService(db, igdbClient, epicClient, enc)

	job, err := svc.StartEpicImport(ctx, user.ID)
	if err != nil {
		t.Fatalf("StartEpicImport() error = %v", err)
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
	if igdbCalls == 0 {
		t.Error("igdb API calls = 0, want metadata backfill for yearless game")
	}
}

func TestStartEpicImportSkipsSoftDeleted(t *testing.T) {
	t.Setenv("TWITCH_CLIENT_ID", "test-client")
	t.Setenv("TWITCH_CLIENT_SECRET", "test-secret")

	const (
		accessToken = "epic-access-token"
		catalogID   = "44444444-4444-4444-4444-444444444444"
		accountID   = "9626f441055349ce8cb7d7d5a483eaa2"
	)

	libraryServer := newMockEpicLibraryServer(t, accessToken, []map[string]any{
		{
			"appName":       "Hades",
			"namespace":     "ns",
			"catalogItemId": catalogID,
			"sandboxType":   "PUBLICGAME",
			"metadata":      map[string]any{"title": "Hades"},
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
	user, enc := setupEpicLinkedUser(t, db, accountID, accessToken)

	cat, err := models.GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}
	existing, err := models.FindOrCreateGameByEpicCatalogItemID(ctx, db, catalogID, "Hades", "https://cdn.example/hades.jpg", cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGameByEpicCatalogItemID() error = %v", err)
	}
	if err := models.AddToLibrary(ctx, db, user.ID, existing.ID, "Epic", nil); err != nil {
		t.Fatalf("AddToLibrary() error = %v", err)
	}
	if err := models.RemoveFromLibrary(ctx, db, user.ID, existing.ID); err != nil {
		t.Fatalf("RemoveFromLibrary() error = %v", err)
	}

	igdbClient := igdb.NewClient("test-client", "test-secret", igdbServer.URL)
	igdbClient.SetTokenURL(igdbServer.URL + "/token")
	igdbClient.SetHTTPClient(igdbServer.Client())

	epicClient := epic.NewClientWithHTTP("client-id", "secret", libraryServer.Client())
	epicClient.SetLibraryURL(libraryServer.URL)

	svc := newEpicImportService(db, igdbClient, epicClient, enc)

	job, err := svc.StartEpicImport(ctx, user.ID)
	if err != nil {
		t.Fatalf("StartEpicImport() error = %v", err)
	}
	waitForImportJob(t, db, job.ID, 5*time.Second)

	got, err := models.GetImportJob(ctx, db, job.ID)
	if err != nil {
		t.Fatalf("GetImportJob() error = %v", err)
	}
	if got.ImportedCount != 0 {
		t.Errorf("imported_count = %d, want 0", got.ImportedCount)
	}
	if igdbCalls != 0 {
		t.Errorf("igdb API calls = %d, want 0 for soft-deleted game", igdbCalls)
	}

	games, err := models.ListUserGames(ctx, db, user.ID)
	if err != nil {
		t.Fatalf("ListUserGames() error = %v", err)
	}
	if len(games) != 0 {
		t.Fatalf("library count = %d, want 0 after soft-deleted Epic sync", len(games))
	}
}

func TestStartEpicImportRestartsStaleJob(t *testing.T) {
	t.Setenv("TWITCH_CLIENT_ID", "test-client")
	t.Setenv("TWITCH_CLIENT_SECRET", "test-secret")

	const (
		accessToken = "epic-access-token"
		accountID   = "9626f441055349ce8cb7d7d5a483eaa2"
	)

	libraryServer := newMockEpicLibraryServer(t, accessToken, []map[string]any{})
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
	user, enc := setupEpicLinkedUser(t, db, accountID, accessToken)

	stale, err := models.CreateImportJob(ctx, db, user.ID, "epic")
	if err != nil {
		t.Fatalf("CreateImportJob() error = %v", err)
	}

	igdbClient := igdb.NewClient("test-client", "test-secret", igdbServer.URL)
	igdbClient.SetTokenURL(igdbServer.URL + "/token")
	igdbClient.SetHTTPClient(igdbServer.Client())

	epicClient := epic.NewClientWithHTTP("client-id", "secret", libraryServer.Client())
	epicClient.SetLibraryURL(libraryServer.URL)

	svc := newEpicImportService(db, igdbClient, epicClient, enc)

	job, err := svc.StartEpicImport(ctx, user.ID)
	if err != nil {
		t.Fatalf("StartEpicImport() error = %v", err)
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

func TestRunEpicImportWithoutIGDBCredentials(t *testing.T) {
	t.Setenv("TWITCH_CLIENT_ID", "")
	t.Setenv("TWITCH_CLIENT_SECRET", "")

	const (
		accessToken = "epic-access-token"
		catalogID   = "55555555-5555-5555-5555-555555555555"
		accountID   = "9626f441055349ce8cb7d7d5a483eaa2"
	)

	libraryServer := newMockEpicLibraryServer(t, accessToken, []map[string]any{
		{
			"appName":       "Epic Only Game",
			"namespace":     "ns",
			"catalogItemId": catalogID,
			"sandboxType":   "PUBLICGAME",
			"metadata":      map[string]any{"title": "Epic Only Game"},
		},
	})

	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	user, enc := setupEpicLinkedUser(t, db, accountID, accessToken)

	epicClient := epic.NewClientWithHTTP("client-id", "secret", libraryServer.Client())
	epicClient.SetLibraryURL(libraryServer.URL)

	svc := newEpicImportService(db, igdb.NewClient("", "", "http://localhost"), epicClient, enc)

	job, err := svc.StartEpicImport(ctx, user.ID)
	if err != nil {
		t.Fatalf("StartEpicImport() error = %v", err)
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

func TestStartEpicImportRecordsAPIFailure(t *testing.T) {
	t.Setenv("TWITCH_CLIENT_ID", "")
	t.Setenv("TWITCH_CLIENT_SECRET", "")

	const (
		accessToken = "epic-access-token"
		accountID   = "9626f441055349ce8cb7d7d5a483eaa2"
	)

	libraryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errorCode":"errors.com.epicgames.common.authentication.authentication_failed"}`))
	}))
	defer libraryServer.Close()

	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	user, enc := setupEpicLinkedUser(t, db, accountID, accessToken)

	epicClient := epic.NewClientWithHTTP("client-id", "secret", libraryServer.Client())
	epicClient.SetLibraryURL(libraryServer.URL)

	svc := newEpicImportService(db, igdb.NewClient("", "", "http://localhost"), epicClient, enc)

	job, err := svc.StartEpicImport(ctx, user.ID)
	if err != nil {
		t.Fatalf("StartEpicImport() error = %v", err)
	}

	waitForImportJob(t, db, job.ID, 5*time.Second)

	got, err := models.GetImportJob(ctx, db, job.ID)
	if err != nil {
		t.Fatalf("GetImportJob() error = %v", err)
	}
	if got.Status != "failed" {
		t.Fatalf("status = %q, want failed", got.Status)
	}
	if got.ErrorMessage == "" || !strings.Contains(got.ErrorMessage, "re-link") {
		t.Errorf("error_message = %q, want API failure details", got.ErrorMessage)
	}
}

func newMockEpicLibraryServer(t *testing.T, accessToken string, records []map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+accessToken {
			t.Errorf("Authorization = %q, want bearer access token", got)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"records":          records,
			"responseMetadata": map[string]any{"nextCursor": nil},
		})
	}))
}

func setupEpicLinkedUser(t *testing.T, db *pgxpool.Pool, accountID, accessToken string) (*models.User, *crypto.Encrypter) {
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
		ctx, db, user.ID, "epic", accountID, "EpicGamer",
		accessEnc, refreshEnc, &expiresAt,
	); err != nil {
		t.Fatalf("UpsertLinkedAccount() error = %v", err)
	}

	return user, enc
}
