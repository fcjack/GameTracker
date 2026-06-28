package igdb

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReleaseYear(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		timestamp int64
		want      int
	}{
		{"zero timestamp", 0, 0},
		{"known date", time.Date(2020, 6, 15, 0, 0, 0, 0, time.UTC).Unix(), 2020},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ReleaseYear(tt.timestamp); got != tt.want {
				t.Errorf("ReleaseYear() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestNormalizeCoverURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"empty", "", ""},
		{"protocol-relative", "//images.igdb.com/igdb/image/upload/t_thumb/co1abc.jpg", "https://images.igdb.com/igdb/image/upload/t_cover_big/co1abc.jpg"},
		{"already https", "https://images.igdb.com/igdb/image/upload/t_thumb/co1abc.jpg", "https://images.igdb.com/igdb/image/upload/t_cover_big/co1abc.jpg"},
		{"no thumb token", "https://example.com/cover.jpg", "https://example.com/cover.jpg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeCoverURL(tt.raw); got != tt.want {
				t.Errorf("normalizeCoverURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSearch(t *testing.T) {
	t.Parallel()
	const testToken = "test-access-token"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/token":
			if got := r.FormValue("client_id"); got != "test-client" {
				t.Errorf("token request client_id = %q, want test-client", got)
			}
			if got := r.FormValue("grant_type"); got != "client_credentials" {
				t.Errorf("token request grant_type = %q, want client_credentials", got)
			}
			json.NewEncoder(w).Encode(tokenResponse{AccessToken: testToken, ExpiresIn: 3600})

		case r.Method == http.MethodPost && r.URL.Path == "/games":
			if got := r.Header.Get("Authorization"); got != "Bearer "+testToken {
				t.Errorf("Authorization = %q, want Bearer %s", got, testToken)
			}
			if got := r.Header.Get("Client-ID"); got != "test-client" {
				t.Errorf("Client-ID = %q, want test-client", got)
			}
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `search "Halo"`) {
				t.Errorf("search body = %q, expected Halo query", string(body))
			}
			json.NewEncoder(w).Encode([]SearchResult{
				{
					ID:               1234,
					Name:             "Halo: Combat Evolved",
					Category:         0,
					FirstReleaseDate: time.Date(2001, 11, 15, 0, 0, 0, 0, time.UTC).Unix(),
					Cover:            &Cover{URL: "//images.igdb.com/igdb/image/upload/t_thumb/co1abc.jpg"},
					Platforms:        []Platform{{ID: 6, Name: "PC (Microsoft Windows)"}},
				},
			})

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient("test-client", "test-secret", server.URL)
	client.tokenURL = server.URL + "/token"
	client.httpClient = server.Client()

	results, err := client.Search("Halo", 5)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Search() returned %d results, want 1", len(results))
	}

	got := results[0]
	if got.Name != "Halo: Combat Evolved" {
		t.Errorf("result name = %q, want Halo: Combat Evolved", got.Name)
	}
	if got.Cover == nil || got.Cover.URL != "https://images.igdb.com/igdb/image/upload/t_cover_big/co1abc.jpg" {
		t.Errorf("cover URL not normalized: %+v", got.Cover)
	}
}

func TestSearchEscapesQuotes(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			json.NewEncoder(w).Encode(tokenResponse{AccessToken: "tok", ExpiresIn: 3600})
		case "/games":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `search "foo\"bar"`) {
				t.Errorf("search body = %q, expected escaped quotes", string(body))
			}
			json.NewEncoder(w).Encode([]SearchResult{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient("id", "secret", server.URL)
	client.tokenURL = server.URL + "/token"
	client.httpClient = server.Client()

	_, err := client.Search(`foo"bar`, 1)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
}

func TestSearchTokenCaching(t *testing.T) {
	t.Parallel()
	tokenRequests := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			tokenRequests++
			json.NewEncoder(w).Encode(tokenResponse{AccessToken: "cached-token", ExpiresIn: 3600})
		case "/games":
			json.NewEncoder(w).Encode([]SearchResult{{ID: 1, Name: "Game"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient("id", "secret", server.URL)
	client.tokenURL = server.URL + "/token"
	client.httpClient = server.Client()

	if _, err := client.Search("a", 1); err != nil {
		t.Fatalf("first Search() error = %v", err)
	}
	if _, err := client.Search("b", 1); err != nil {
		t.Fatalf("second Search() error = %v", err)
	}

	if tokenRequests != 1 {
		t.Errorf("token endpoint called %d times, want 1 (cached)", tokenRequests)
	}
}

func TestSearchUnauthorizedClearsToken(t *testing.T) {
	t.Parallel()
	tokenRequests := 0
	searchRequests := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			tokenRequests++
			json.NewEncoder(w).Encode(tokenResponse{AccessToken: "short-lived", ExpiresIn: 3600})
		case "/games":
			searchRequests++
			if searchRequests == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			json.NewEncoder(w).Encode([]SearchResult{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient("id", "secret", server.URL)
	client.tokenURL = server.URL + "/token"
	client.httpClient = server.Client()

	_, err := client.Search("query", 1)
	if err == nil || !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("expected unauthorized error, got %v", err)
	}

	// Token should have been cleared; a new token request is needed on retry.
	if _, err := client.Search("query", 1); err != nil {
		t.Fatalf("retry Search() error = %v", err)
	}

	if tokenRequests != 2 {
		t.Errorf("token endpoint called %d times, want 2 after 401 invalidation", tokenRequests)
	}
}

func TestLookupIGDBIDBySteamAppID(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			json.NewEncoder(w).Encode(tokenResponse{AccessToken: "tok", ExpiresIn: 3600})
		case "/games":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `external_games.uid = "730"`) {
				t.Errorf("games body = %q, expected steam app id 730", string(body))
			}
			json.NewEncoder(w).Encode([]SearchResult{{ID: 12345, Name: "Counter-Strike 2"}})
		case "/external_games":
			json.NewEncoder(w).Encode([]externalGameResult{{Game: 99999}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient("id", "secret", server.URL)
	client.tokenURL = server.URL + "/token"
	client.httpClient = server.Client()

	igdbID, err := client.LookupIGDBIDBySteamAppID(730, "Counter-Strike 2")
	if err != nil {
		t.Fatalf("LookupIGDBIDBySteamAppID() error = %v", err)
	}
	if igdbID != 12345 {
		t.Errorf("igdb id = %d, want 12345", igdbID)
	}
}

func TestLookupIGDBIDBySteamAppIDFallback(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			json.NewEncoder(w).Encode(tokenResponse{AccessToken: "tok", ExpiresIn: 3600})
		case "/games":
			json.NewEncoder(w).Encode([]SearchResult{})
		case "/external_games":
			json.NewEncoder(w).Encode([]externalGameResult{
				{Game: 999, Category: 3},
				{Game: 2963, Category: 0},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient("id", "secret", server.URL)
	client.SetTokenURL(server.URL + "/token")
	client.SetHTTPClient(server.Client())

	igdbID, err := client.LookupIGDBIDBySteamAppID(570, "Dota 2")
	if err != nil {
		t.Fatalf("LookupIGDBIDBySteamAppID() error = %v", err)
	}
	if igdbID != 2963 {
		t.Errorf("igdb id = %d, want 2963", igdbID)
	}
}

func TestLookupIGDBIDByXboxTitleID(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			json.NewEncoder(w).Encode(tokenResponse{AccessToken: "tok", ExpiresIn: 3600})
		case "/games":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `external_games.uid = "1144039928"`) {
				t.Errorf("games body = %q, expected xbox title id 1144039928", string(body))
			}
			json.NewEncoder(w).Encode([]SearchResult{{ID: 98765, Name: "Halo Infinite"}})
		case "/external_games":
			json.NewEncoder(w).Encode([]externalGameResult{{Game: 11111}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient("id", "secret", server.URL)
	client.tokenURL = server.URL + "/token"
	client.httpClient = server.Client()

	igdbID, err := client.LookupIGDBIDByXboxTitleID(1144039928, "Halo Infinite")
	if err != nil {
		t.Fatalf("LookupIGDBIDByXboxTitleID() error = %v", err)
	}
	if igdbID != 98765 {
		t.Errorf("igdb id = %d, want 98765", igdbID)
	}
}

func TestLookupIGDBIDByXboxTitleIDExternalGamesFallback(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			json.NewEncoder(w).Encode(tokenResponse{AccessToken: "tok", ExpiresIn: 3600})
		case "/games":
			body, _ := io.ReadAll(r.Body)
			if strings.Contains(string(body), `search "Halo Infinite"`) {
				json.NewEncoder(w).Encode([]SearchResult{})
				return
			}
			json.NewEncoder(w).Encode([]SearchResult{})
		case "/external_games":
			json.NewEncoder(w).Encode([]externalGameResult{
				{Game: 999, Category: 3, Name: "Wrong Game"},
				{Game: 135590, Category: externalGameCategoryXboxMarketplace, Name: "Halo Infinite"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient("id", "secret", server.URL)
	client.SetTokenURL(server.URL + "/token")
	client.SetHTTPClient(server.Client())

	igdbID, err := client.LookupIGDBIDByXboxTitleID(1144039928, "Halo Infinite")
	if err != nil {
		t.Fatalf("LookupIGDBIDByXboxTitleID() error = %v", err)
	}
	if igdbID != 135590 {
		t.Errorf("igdb id = %d, want 135590", igdbID)
	}
}

func TestLookupIGDBIDByXboxTitleIDNameSearchFallback(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			json.NewEncoder(w).Encode(tokenResponse{AccessToken: "tok", ExpiresIn: 3600})
		case "/games":
			body, _ := io.ReadAll(r.Body)
			if strings.Contains(string(body), `search "Ori and the Will of the Wisps"`) {
				json.NewEncoder(w).Encode([]SearchResult{
					{ID: 119171, Name: "Ori and the Will of the Wisps", Category: 0},
				})
				return
			}
			json.NewEncoder(w).Encode([]SearchResult{})
		case "/external_games":
			json.NewEncoder(w).Encode([]externalGameResult{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient("id", "secret", server.URL)
	client.SetTokenURL(server.URL + "/token")
	client.httpClient = server.Client()

	igdbID, err := client.LookupIGDBIDByXboxTitleID(1659804324, "Ori and the Will of the Wisps")
	if err != nil {
		t.Fatalf("LookupIGDBIDByXboxTitleID() error = %v", err)
	}
	if igdbID != 119171 {
		t.Errorf("igdb id = %d, want 119171", igdbID)
	}
}

func TestLookupIGDBIDByXboxTitleIDWithoutCredentialsReturnsZero(t *testing.T) {
	t.Parallel()
	client := NewClient("", "", "http://example.invalid")

	igdbID, err := client.LookupIGDBIDByXboxTitleID(1144039928, "Halo Infinite")
	if err == nil {
		t.Fatalf("LookupIGDBIDByXboxTitleID() error = nil, want token failure")
	}
	if igdbID != 0 {
		t.Errorf("igdb id = %d, want 0 on lookup failure", igdbID)
	}
}

func TestGetGameByID(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			json.NewEncoder(w).Encode(tokenResponse{AccessToken: "tok", ExpiresIn: 3600})
		case "/games":
			json.NewEncoder(w).Encode([]SearchResult{
				{
					ID:               99,
					Name:             "Portal",
					Category:         0,
					FirstReleaseDate: time.Date(2007, 10, 10, 0, 0, 0, 0, time.UTC).Unix(),
					Cover:            &Cover{URL: "//images.igdb.com/igdb/image/upload/t_thumb/co99.jpg"},
					Platforms:        []Platform{{Name: "PC (Microsoft Windows)"}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient("id", "secret", server.URL)
	client.tokenURL = server.URL + "/token"
	client.httpClient = server.Client()

	game, err := client.GetGameByID(99)
	if err != nil {
		t.Fatalf("GetGameByID() error = %v", err)
	}
	if game == nil || game.Name != "Portal" {
		t.Fatalf("GetGameByID() = %+v, want Portal", game)
	}
}

func TestSearchTokenRequestFailure(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := NewClient("id", "secret", server.URL)
	client.tokenURL = server.URL + "/token"
	client.httpClient = server.Client()

	_, err := client.Search("query", 1)
	if err == nil || !strings.Contains(err.Error(), "token request returned 403") {
		t.Fatalf("expected token failure error, got %v", err)
	}
}
