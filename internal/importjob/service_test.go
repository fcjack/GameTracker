package importjob

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jacksoncoelho/game-tracker/internal/database"
	"github.com/jacksoncoelho/game-tracker/internal/igdb"
	"github.com/jacksoncoelho/game-tracker/internal/models"
	"github.com/jacksoncoelho/game-tracker/internal/steam"
)

func TestStartSteamImportImportsGames(t *testing.T) {
	t.Setenv("TWITCH_CLIENT_ID", "test-client")
	t.Setenv("TWITCH_CLIENT_SECRET", "test-secret")

	const steamID = "76561198012345678"

	steamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "GetOwnedGames") {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"response": map[string]any{
				"game_count": 2,
				"games": []map[string]any{
					{"appid": 570, "name": "Dota 2", "img_icon_url": "dota_icon", "playtime_forever": 150},
					{"appid": 99999, "name": "Unknown Game", "img_icon_url": "unknown_icon", "playtime_forever": 45},
				},
			},
		})
	}))
	defer steamServer.Close()

	igdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
		case "/games":
			body, _ := io.ReadAll(r.Body)
			text := string(body)
			switch {
			case strings.Contains(text, `external_games.uid = "570"`):
				json.NewEncoder(w).Encode([]map[string]any{{"id": 2963, "name": "Dota 2"}})
			case strings.Contains(text, "where id = 2963"):
				json.NewEncoder(w).Encode([]map[string]any{
					{
						"id":                 2963,
						"name":               "Dota 2",
						"category":           0,
						"first_release_date": time.Date(2013, 7, 9, 0, 0, 0, 0, time.UTC).Unix(),
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
	user, err := models.CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	igdbClient := igdb.NewClient("test-client", "test-secret", igdbServer.URL)
	igdbClient.SetTokenURL(igdbServer.URL + "/token")
	igdbClient.SetHTTPClient(igdbServer.Client())

	steamClient := steam.NewClientWithHTTP("steam-key", steamServer.URL, steamServer.Client())
	storeClient := testStoreClient(t, map[int]string{570: "game", 99999: "game"})
	svc := newSteamImportService(db, igdbClient, steamClient, storeClient)

	job, err := svc.StartSteamImport(ctx, user.ID, steamID)
	if err != nil {
		t.Fatalf("StartSteamImport() error = %v", err)
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
	if got.SkippedCount != 0 {
		t.Errorf("skipped_count = %d, want 0", got.SkippedCount)
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
	if dota := byName["Dota 2"]; dota == nil || dota.Platform != "Steam" || dota.IGDBId == nil {
		t.Errorf("Dota 2 = %+v, want Steam platform with IGDB id", dota)
	} else if dota.PlaytimeMinutes == nil || *dota.PlaytimeMinutes != 150 {
		t.Errorf("Dota 2 playtime = %v, want 150", dota.PlaytimeMinutes)
	}
	if unknown := byName["Unknown Game"]; unknown == nil || unknown.Platform != "Steam" {
		t.Errorf("Unknown Game = %+v, want Steam platform", unknown)
	} else if unknown.IGDBId != nil {
		t.Errorf("Unknown Game igdb_id = %v, want nil", *unknown.IGDBId)
	} else if unknown.CoverURL == "" {
		t.Error("Unknown Game cover_url should use Steam CDN")
	} else if unknown.PlaytimeMinutes == nil || *unknown.PlaytimeMinutes != 45 {
		t.Errorf("Unknown Game playtime = %v, want 45", unknown.PlaytimeMinutes)
	}
}

func TestStartSteamImportSkipsAlreadyImported(t *testing.T) {
	t.Setenv("TWITCH_CLIENT_ID", "test-client")
	t.Setenv("TWITCH_CLIENT_SECRET", "test-secret")

	const steamID = "76561198012345678"

	steamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"response": map[string]any{
				"game_count": 1,
				"games": []map[string]any{
					{"appid": 570, "name": "Dota 2", "img_icon_url": "dota_icon", "playtime_forever": 8520},
				},
			},
		})
	}))
	defer steamServer.Close()

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
	user, err := models.CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	cat, err := models.GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}
	existing, err := models.FindOrCreateGameBySteamAppID(ctx, db, 570, "Dota 2", "https://cdn.example.com/dota.jpg", cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGameBySteamAppID() error = %v", err)
	}
	if err := models.AddToLibrary(ctx, db, user.ID, existing.ID, "Steam", nil); err != nil {
		t.Fatalf("AddToLibrary() error = %v", err)
	}

	igdbClient := igdb.NewClient("test-client", "test-secret", igdbServer.URL)
	igdbClient.SetTokenURL(igdbServer.URL + "/token")
	igdbClient.SetHTTPClient(igdbServer.Client())
	storeClient := testStoreClient(t, map[int]string{570: "game"})
	svc := newSteamImportService(db, igdbClient, steam.NewClientWithHTTP("key", steamServer.URL, steamServer.Client()), storeClient)

	job, err := svc.StartSteamImport(ctx, user.ID, steamID)
	if err != nil {
		t.Fatalf("StartSteamImport() error = %v", err)
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

	games, err := models.ListUserGames(ctx, db, user.ID)
	if err != nil {
		t.Fatalf("ListUserGames() error = %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("library count = %d, want 1", len(games))
	}
	if games[0].PlaytimeMinutes == nil || *games[0].PlaytimeMinutes != 8520 {
		t.Fatalf("playtime after re-sync = %v, want 8520", games[0].PlaytimeMinutes)
	}
}

func TestStartSteamImportSkipsSoftDeleted(t *testing.T) {
	t.Setenv("TWITCH_CLIENT_ID", "test-client")
	t.Setenv("TWITCH_CLIENT_SECRET", "test-secret")

	const steamID = "76561198012345678"

	steamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"response": map[string]any{
				"game_count": 1,
				"games": []map[string]any{
					{"appid": 570, "name": "Dota 2", "img_icon_url": "dota_icon", "playtime_forever": 8520},
				},
			},
		})
	}))
	defer steamServer.Close()

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
	user, err := models.CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	cat, err := models.GetCategoryByIGDBValue(ctx, db, 0)
	if err != nil {
		t.Fatalf("GetCategoryByIGDBValue() error = %v", err)
	}
	existing, err := models.FindOrCreateGameBySteamAppID(ctx, db, 570, "Dota 2", "https://cdn.example.com/dota.jpg", cat.ID)
	if err != nil {
		t.Fatalf("FindOrCreateGameBySteamAppID() error = %v", err)
	}
	if err := models.AddToLibrary(ctx, db, user.ID, existing.ID, "Steam", nil); err != nil {
		t.Fatalf("AddToLibrary() error = %v", err)
	}
	if err := models.RemoveFromLibrary(ctx, db, user.ID, existing.ID); err != nil {
		t.Fatalf("RemoveFromLibrary() error = %v", err)
	}

	igdbClient := igdb.NewClient("test-client", "test-secret", igdbServer.URL)
	igdbClient.SetTokenURL(igdbServer.URL + "/token")
	igdbClient.SetHTTPClient(igdbServer.Client())
	storeClient := testStoreClient(t, map[int]string{570: "game"})
	svc := newSteamImportService(db, igdbClient, steam.NewClientWithHTTP("key", steamServer.URL, steamServer.Client()), storeClient)

	job, err := svc.StartSteamImport(ctx, user.ID, steamID)
	if err != nil {
		t.Fatalf("StartSteamImport() error = %v", err)
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
		t.Fatalf("library count = %d, want 0 after soft-deleted Steam sync", len(games))
	}
}

func TestStartSteamImportReturnsExistingActiveJob(t *testing.T) {
	t.Parallel()
	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	user, err := models.CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	existing, err := models.CreateImportJob(ctx, db, user.ID, "steam")
	if err != nil {
		t.Fatalf("CreateImportJob() error = %v", err)
	}
	if err := models.SetImportJobTotal(ctx, db, existing.ID, 5); err != nil {
		t.Fatalf("SetImportJobTotal() error = %v", err)
	}
	if err := models.UpdateImportJobProgress(ctx, db, existing.ID, 2, 1, 1); err != nil {
		t.Fatalf("UpdateImportJobProgress() error = %v", err)
	}

	svc := newSteamImportService(db, igdb.NewClient("id", "secret", "http://localhost"), steam.NewClient("key"), testStoreClient(t, nil))
	job, err := svc.StartSteamImport(ctx, user.ID, "76561198012345678")
	if err != nil {
		t.Fatalf("StartSteamImport() error = %v", err)
	}
	if job.ID != existing.ID {
		t.Errorf("job id = %d, want existing %d", job.ID, existing.ID)
	}
}

func TestStartSteamImportRestartsStalePendingJob(t *testing.T) {
	t.Setenv("TWITCH_CLIENT_ID", "test-client")
	t.Setenv("TWITCH_CLIENT_SECRET", "test-secret")

	steamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"response": map[string]any{"game_count": 0, "games": []any{}}})
	}))
	defer steamServer.Close()

	igdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
		case "/games", "/external_games":
			w.Write([]byte("[]"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer igdbServer.Close()

	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	user, err := models.CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	stale, err := models.CreateImportJob(ctx, db, user.ID, "steam")
	if err != nil {
		t.Fatalf("CreateImportJob() error = %v", err)
	}

	igdbClient := igdb.NewClient("test-client", "test-secret", igdbServer.URL)
	igdbClient.SetTokenURL(igdbServer.URL + "/token")
	igdbClient.SetHTTPClient(igdbServer.Client())
	svc := newSteamImportService(db, igdbClient, steam.NewClientWithHTTP("key", steamServer.URL, steamServer.Client()), testStoreClient(t, nil))

	job, err := svc.StartSteamImport(ctx, user.ID, "76561198012345678")
	if err != nil {
		t.Fatalf("StartSteamImport() error = %v", err)
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

func TestRunSteamImportWithoutIGDBCredentials(t *testing.T) {
	t.Setenv("TWITCH_CLIENT_ID", "")
	t.Setenv("TWITCH_CLIENT_SECRET", "")

	steamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"response": map[string]any{
				"game_count": 1,
				"games": []map[string]any{
					{"appid": 123, "name": "Steam Only Game", "img_icon_url": "abc123"},
				},
			},
		})
	}))
	defer steamServer.Close()

	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	user, err := models.CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	svc := newSteamImportService(
		db,
		igdb.NewClient("", "", "http://localhost"),
		steam.NewClientWithHTTP("key", steamServer.URL, steamServer.Client()),
		testStoreClient(t, map[int]string{123: "game"}),
	)
	job, err := svc.StartSteamImport(ctx, user.ID, "76561198012345678")
	if err != nil {
		t.Fatalf("StartSteamImport() error = %v", err)
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

func TestStartSteamImportSkipsNonGameTypes(t *testing.T) {
	t.Setenv("TWITCH_CLIENT_ID", "")
	t.Setenv("TWITCH_CLIENT_SECRET", "")

	steamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"response": map[string]any{
				"game_count": 3,
				"games": []map[string]any{
					{"appid": 570, "name": "Dota 2", "img_icon_url": "dota"},
					{"appid": 401920, "name": "Afterbirth", "img_icon_url": "dlc"},
					{"appid": 3838, "name": "Soundtrack", "img_icon_url": "music"},
				},
			},
		})
	}))
	defer steamServer.Close()

	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	user, err := models.CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	storeClient := testStoreClient(t, map[int]string{
		570:    "game",
		401920: "dlc",
		3838:   "music",
	})
	svc := newSteamImportService(
		db,
		igdb.NewClient("", "", "http://localhost"),
		steam.NewClientWithHTTP("key", steamServer.URL, steamServer.Client()),
		storeClient,
	)

	job, err := svc.StartSteamImport(ctx, user.ID, "76561198012345678")
	if err != nil {
		t.Fatalf("StartSteamImport() error = %v", err)
	}
	waitForImportJob(t, db, job.ID, 5*time.Second)

	got, err := models.GetImportJob(ctx, db, job.ID)
	if err != nil {
		t.Fatalf("GetImportJob() error = %v", err)
	}
	if got.Status != "completed" {
		t.Fatalf("status = %q, want completed (error: %s)", got.Status, got.ErrorMessage)
	}
	if got.TotalCount != 3 {
		t.Errorf("total_count = %d, want 3", got.TotalCount)
	}
	if got.ImportedCount != 1 {
		t.Errorf("imported_count = %d, want 1", got.ImportedCount)
	}
	if got.SkippedCount != 2 {
		t.Errorf("skipped_count = %d, want 2", got.SkippedCount)
	}

	games, err := models.ListUserGames(ctx, db, user.ID)
	if err != nil {
		t.Fatalf("ListUserGames() error = %v", err)
	}
	if len(games) != 1 || games[0].Name != "Dota 2" {
		t.Fatalf("library = %+v, want only Dota 2", games)
	}
}

func TestStartSteamImportPrefersSteamMetadata(t *testing.T) {
	t.Setenv("TWITCH_CLIENT_ID", "test-client")
	t.Setenv("TWITCH_CLIENT_SECRET", "test-secret")

	const steamID = "76561198012345678"

	steamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"response": map[string]any{
				"game_count": 1,
				"games": []map[string]any{
					{"appid": 991730, "name": "Counter-Strike 2", "img_icon_url": "cs_icon"},
				},
			},
		})
	}))
	defer steamServer.Close()

	igdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
		case "/games":
			body, _ := io.ReadAll(r.Body)
			text := string(body)
			switch {
			case strings.Contains(text, `external_games.uid = "991730"`):
				json.NewEncoder(w).Encode([]map[string]any{{"id": 991001, "name": "Counter-Strike 2"}})
			case strings.Contains(text, "where id = 991001"):
				json.NewEncoder(w).Encode([]map[string]any{
					{
						"id":                 991001,
						"name":               "Counter-Strike",
						"category":           0,
						"cover":              map[string]any{"url": "//images.igdb.com/igdb/image/upload/t_thumb/coabc.jpg"},
						"first_release_date": time.Date(2012, 8, 21, 0, 0, 0, 0, time.UTC).Unix(),
						"platforms":          []map[string]any{{"name": "PC (Microsoft Windows)"}},
					},
				})
			default:
				json.NewEncoder(w).Encode([]any{})
			}
		default:
			json.NewEncoder(w).Encode([]any{})
		}
	}))
	defer igdbServer.Close()

	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	user, err := models.CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	igdbClient := igdb.NewClient("test-client", "test-secret", igdbServer.URL)
	igdbClient.SetTokenURL(igdbServer.URL + "/token")
	igdbClient.SetHTTPClient(igdbServer.Client())
	svc := newSteamImportService(
		db, igdbClient,
		steam.NewClientWithHTTP("key", steamServer.URL, steamServer.Client()),
		testStoreClient(t, map[int]string{991730: "game"}),
	)

	job, err := svc.StartSteamImport(ctx, user.ID, steamID)
	if err != nil {
		t.Fatalf("StartSteamImport() error = %v", err)
	}
	waitForImportJob(t, db, job.ID, 5*time.Second)

	games, err := models.ListUserGames(ctx, db, user.ID)
	if err != nil {
		t.Fatalf("ListUserGames() error = %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("library count = %d, want 1", len(games))
	}
	if games[0].Name != "Counter-Strike 2" {
		t.Errorf("name = %q, want Counter-Strike 2 from Steam", games[0].Name)
	}
	if !strings.Contains(games[0].CoverURL, "/991730/") {
		t.Errorf("cover_url = %q, want Steam CDN URL", games[0].CoverURL)
	}
}

func TestStartSteamImportSkipsMismatchedIGDBName(t *testing.T) {
	t.Setenv("TWITCH_CLIENT_ID", "test-client")
	t.Setenv("TWITCH_CLIENT_SECRET", "test-secret")

	const steamID = "76561198012345678"

	steamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"response": map[string]any{
				"game_count": 1,
				"games": []map[string]any{
					{"appid": 9912345, "name": "Steam Title", "img_icon_url": "icon"},
				},
			},
		})
	}))
	defer steamServer.Close()

	igdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
		case "/games":
			body, _ := io.ReadAll(r.Body)
			text := string(body)
			switch {
			case strings.Contains(text, `external_games.uid = "9912345"`):
				json.NewEncoder(w).Encode([]map[string]any{{"id": 992002, "name": "Steam Title"}})
			case strings.Contains(text, "where id = 992002"):
				json.NewEncoder(w).Encode([]map[string]any{
					{
						"id":       992002,
						"name":     "Wrong IGDB Game",
						"category": 0,
					},
				})
			default:
				json.NewEncoder(w).Encode([]any{})
			}
		default:
			json.NewEncoder(w).Encode([]any{})
		}
	}))
	defer igdbServer.Close()

	db := testDB(t)
	defer db.Close()

	ctx := context.Background()
	user, err := models.CreateUser(ctx, db, uniqueUsername(t), "password123")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	igdbClient := igdb.NewClient("test-client", "test-secret", igdbServer.URL)
	igdbClient.SetTokenURL(igdbServer.URL + "/token")
	igdbClient.SetHTTPClient(igdbServer.Client())
	svc := newSteamImportService(
		db, igdbClient,
		steam.NewClientWithHTTP("key", steamServer.URL, steamServer.Client()),
		testStoreClient(t, map[int]string{9912345: "game"}),
	)

	job, err := svc.StartSteamImport(ctx, user.ID, steamID)
	if err != nil {
		t.Fatalf("StartSteamImport() error = %v", err)
	}
	waitForImportJob(t, db, job.ID, 5*time.Second)

	games, err := models.ListUserGames(ctx, db, user.ID)
	if err != nil {
		t.Fatalf("ListUserGames() error = %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("library count = %d, want 1", len(games))
	}
	if games[0].Name != "Steam Title" {
		t.Errorf("name = %q, want Steam Title", games[0].Name)
	}
	if games[0].IGDBId != nil {
		t.Errorf("igdb_id = %v, want nil when IGDB name mismatches", games[0].IGDBId)
	}
}

func testStoreClient(t *testing.T, types map[int]string) *steam.StoreClient {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		appIDStr := r.URL.Query().Get("appids")
		appID, err := strconv.Atoi(appIDStr)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		typ := "game"
		if types != nil {
			var ok bool
			typ, ok = types[appID]
			if !ok {
				http.NotFound(w, r)
				return
			}
		}

		json.NewEncoder(w).Encode(map[string]any{
			appIDStr: map[string]any{
				"success": true,
				"data":    map[string]any{"type": typ},
			},
		})
	}))
	t.Cleanup(server.Close)

	client := steam.NewStoreClientWithHTTP(server.URL, server.Client())
	client.SetMinInterval(0)
	return client
}

func waitForImportJob(t *testing.T, db *pgxpool.Pool, jobID int64, timeout time.Duration) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		job, err := models.GetImportJob(ctx, db, jobID)
		if err != nil {
			t.Fatalf("GetImportJob() error = %v", err)
		}
		if job.Status == "completed" || job.Status == "failed" {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("import job %d did not finish within %s", jobID, timeout)
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

	db, err := database.Connect(withBoundedPool(dbURL))
	if err != nil {
		t.Skipf("database not available: %v", err)
	}
	if err := database.RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations() error = %v", err)
	}
	return db
}

// withBoundedPool caps the per-test connection pool so parallel tests do not
// exhaust the database's connection limit. Each test only needs a couple of
// connections, and concurrency is already bounded by -parallel.
func withBoundedPool(dbURL string) string {
	if strings.Contains(dbURL, "pool_max_conns") {
		return dbURL
	}
	sep := "?"
	if strings.Contains(dbURL, "?") {
		sep = "&"
	}
	return dbURL + sep + "pool_max_conns=4"
}

var moduleRootOnce sync.Once

// chdirToModuleRoot moves to the module root exactly once per package test
// binary. Using sync.Once keeps it safe to call from parallel tests, since
// os.Chdir mutates process-global state.
func chdirToModuleRoot(t *testing.T) {
	t.Helper()
	moduleRootOnce.Do(func() {
		dir, err := os.Getwd()
		if err != nil {
			panic(err)
		}
		for {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				if err := os.Chdir(dir); err != nil {
					panic(err)
				}
				return
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				panic("could not find module root")
			}
			dir = parent
		}
	})
}

func uniqueUsername(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("importsvc_%d", time.Now().UnixNano())
}
