package steam

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetOwnedGames(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/IPlayerService/GetOwnedGames/v0001/" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"response": map[string]any{
				"game_count": 2,
				"games": []map[string]any{
					{"appid": 730, "name": "Counter-Strike 2"},
					{"appid": 570, "name": "Dota 2"},
				},
			},
		})
	}))
	defer server.Close()

	client := &Client{
		apiKey:     "test-key",
		baseURL:    server.URL,
		httpClient: server.Client(),
	}

	games, err := client.GetOwnedGames("76561198012345678")
	if err != nil {
		t.Fatalf("GetOwnedGames() error = %v", err)
	}
	if len(games) != 2 {
		t.Fatalf("GetOwnedGames() returned %d games, want 2", len(games))
	}
	if games[0].AppID != 730 || games[0].Name != "Counter-Strike 2" {
		t.Errorf("first game = %+v, want CS2", games[0])
	}
}

func TestCoverImageURL(t *testing.T) {
	withIcon := CoverImageURL(570, "7d5a243f9500d2f8467312822f8af2a2928777ed")
	if !strings.Contains(withIcon, "/570/7d5a243f9500d2f8467312822f8af2a2928777ed.jpg") {
		t.Errorf("icon cover = %q", withIcon)
	}

	fallback := CoverImageURL(570, "")
	if !strings.Contains(fallback, "/570/library_600x900.jpg") {
		t.Errorf("fallback cover = %q", fallback)
	}
}

func TestNewClientWithHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"response": map[string]any{
				"games": []map[string]any{{"appid": 1, "name": "Game"}},
			},
		})
	}))
	defer server.Close()

	client := NewClientWithHTTP("key", server.URL, server.Client())
	games, err := client.GetOwnedGames("123")
	if err != nil {
		t.Fatalf("GetOwnedGames() error = %v", err)
	}
	if len(games) != 1 || games[0].AppID != 1 {
		t.Fatalf("games = %+v, want one game with appid 1", games)
	}
}

func TestGetOwnedGamesMissingAPIKey(t *testing.T) {
	client := NewClient("")
	_, err := client.GetOwnedGames("76561198012345678")
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}
