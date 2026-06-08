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

func TestGetOwnedGamesMissingAPIKey(t *testing.T) {
	client := NewClient("")
	_, err := client.GetOwnedGames("76561198012345678")
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}
