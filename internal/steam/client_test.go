package steam

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetOwnedGames(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/IPlayerService/GetOwnedGames/v0001/" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("include_played_free_games"); got != "0" {
			t.Errorf("include_played_free_games = %q, want 0", got)
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
	want := "https://cdn.cloudflare.steamstatic.com/steam/apps/570/library_600x900.jpg"

	withIcon := CoverImageURL(570, "7d5a243f9500d2f8467312822f8af2a2928777ed")
	if withIcon != want {
		t.Errorf("with icon hash = %q, want %q", withIcon, want)
	}

	withoutIcon := CoverImageURL(570, "")
	if withoutIcon != want {
		t.Errorf("without icon hash = %q, want %q", withoutIcon, want)
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
