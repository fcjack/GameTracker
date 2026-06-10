package steam

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestIsImportableAppType(t *testing.T) {
	tests := []struct {
		typ  string
		want bool
	}{
		{"game", true},
		{"dlc", false},
		{"demo", false},
		{"music", false},
		{"software", false},
		{"video", false},
		{"mod", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := IsImportableAppType(tt.typ); got != tt.want {
			t.Errorf("IsImportableAppType(%q) = %v, want %v", tt.typ, got, tt.want)
		}
	}
}

func TestStoreClientGetAppType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("appids") != "570" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"570": map[string]any{
				"success": true,
				"data":    map[string]any{"type": "game"},
			},
		})
	}))
	defer server.Close()

	client := NewStoreClientWithHTTP(server.URL, server.Client())
	client.SetMinInterval(0)

	typ, ok, err := client.GetAppType(context.Background(), 570)
	if err != nil {
		t.Fatalf("GetAppType() error = %v", err)
	}
	if !ok {
		t.Fatal("GetAppType() ok = false, want true")
	}
	if typ != "game" {
		t.Errorf("type = %q, want game", typ)
	}
}

func TestStoreClientGetAppTypeUsesCache(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		json.NewEncoder(w).Encode(map[string]any{
			"570": map[string]any{
				"success": true,
				"data":    map[string]any{"type": "game"},
			},
		})
	}))
	defer server.Close()

	client := NewStoreClientWithHTTP(server.URL, server.Client())
	client.SetMinInterval(0)

	ctx := context.Background()
	if _, _, err := client.GetAppType(ctx, 570); err != nil {
		t.Fatalf("first GetAppType() error = %v", err)
	}
	if _, _, err := client.GetAppType(ctx, 570); err != nil {
		t.Fatalf("second GetAppType() error = %v", err)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("store requests = %d, want 1 (cached second lookup)", got)
	}
}

func TestStoreClientFilterImportableGames(t *testing.T) {
	types := map[int]string{
		570:    "game",
		401920: "dlc",
		3838:   "music",
		601150: "demo",
		228980: "software",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		appID, err := strconv.Atoi(r.URL.Query().Get("appids"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		typ, exists := types[appID]
		if !exists {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			strconv.Itoa(appID): map[string]any{
				"success": true,
				"data":    map[string]any{"type": typ},
			},
		})
	}))
	defer server.Close()

	client := NewStoreClientWithHTTP(server.URL, server.Client())
	client.SetMinInterval(0)

	games := []OwnedGame{
		{AppID: 570, Name: "Dota 2"},
		{AppID: 401920, Name: "Afterbirth"},
		{AppID: 3838, Name: "Soundtrack"},
		{AppID: 601150, Name: "Demo"},
		{AppID: 228980, Name: "Steamworks"},
	}

	filtered, err := client.FilterImportableGames(context.Background(), games)
	if err != nil {
		t.Fatalf("FilterImportableGames() error = %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("filtered count = %d, want 1", len(filtered))
	}
	if filtered[0].AppID != 570 {
		t.Errorf("filtered appid = %d, want 570", filtered[0].AppID)
	}
}

func TestStoreClientFilterImportableGamesParallel(t *testing.T) {
	const apps = 12
	var peakConcurrent atomic.Int32
	var inFlight atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			peak := peakConcurrent.Load()
			if current <= peak || peakConcurrent.CompareAndSwap(peak, current) {
				break
			}
		}

		time.Sleep(25 * time.Millisecond)

		appID := r.URL.Query().Get("appids")
		json.NewEncoder(w).Encode(map[string]any{
			appID: map[string]any{
				"success": true,
				"data":    map[string]any{"type": "game"},
			},
		})
	}))
	defer server.Close()

	client := NewStoreClientWithHTTP(server.URL, server.Client())
	client.SetMinInterval(0)

	games := make([]OwnedGame, apps)
	for i := range games {
		games[i] = OwnedGame{AppID: 1000 + i, Name: "Game"}
	}

	start := time.Now()
	filtered, err := client.FilterImportableGames(context.Background(), games)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("FilterImportableGames() error = %v", err)
	}
	if len(filtered) != apps {
		t.Fatalf("filtered count = %d, want %d", len(filtered), apps)
	}
	if peak := peakConcurrent.Load(); peak < 2 {
		t.Errorf("peak concurrent requests = %d, want at least 2", peak)
	}
	// Sequential would take ~12 * 25ms = 300ms; parallel should finish much faster.
	if elapsed > 200*time.Millisecond {
		t.Errorf("elapsed = %s, want well under 200ms with parallel lookups", elapsed)
	}
}
