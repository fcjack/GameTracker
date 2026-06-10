package steam

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
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

func TestStoreClientFilterImportableGames(t *testing.T) {
	types := map[int]string{
		570:    "game",
		401920: "dlc",
		3838:   "music",
		601150: "demo",
		228980: "software",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		appID := r.URL.Query().Get("appids")
		typ, exists := types[mustAtoi(appID)]
		if !exists {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			appID: map[string]any{
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

func mustAtoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
