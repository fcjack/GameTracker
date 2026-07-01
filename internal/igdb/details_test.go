package igdb

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetGameDetails(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			json.NewEncoder(w).Encode(tokenResponse{AccessToken: "tok", ExpiresIn: 3600})
		case "/games":
			json.NewEncoder(w).Encode([]GameDetails{
				{
					ID:               42,
					Name:             "Hades",
					Summary:          "Battle out of hell.",
					AggregatedRating: 93,
					TotalRating:      96,
					RatingCount:      10,
					TotalRatingCount: 500,
					FirstReleaseDate: time.Date(2020, 9, 17, 0, 0, 0, 0, time.UTC).Unix(),
					GameStatus:       0,
					Genres:           []NamedEntity{{Name: "Roguelike"}},
					Cover:            &Cover{URL: "//images.igdb.com/igdb/image/upload/t_thumb/co42.jpg"},
					Artworks:         []ImageAsset{{URL: "//images.igdb.com/igdb/image/upload/t_thumb/ar1.jpg"}},
					InvolvedCompanies: []InvolvedCompany{{
						Company: struct {
							Name string `json:"name"`
						}{Name: "Supergiant Games"},
						Developer: true,
						Publisher: true,
					}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient("id", "secret", server.URL)
	client.SetTokenURL(server.URL + "/token")
	client.httpClient = server.Client()

	details, err := client.GetGameDetails(42)
	if err != nil {
		t.Fatalf("GetGameDetails() error = %v", err)
	}
	if details == nil || details.Name != "Hades" {
		t.Fatalf("GetGameDetails() = %+v, want Hades", details)
	}
	if details.Summary != "Battle out of hell." {
		t.Errorf("summary = %q", details.Summary)
	}
	if BackdropURL(details) == "" {
		t.Error("BackdropURL() should prefer artwork")
	}
	if len(details.DeveloperNames()) != 1 || details.DeveloperNames()[0] != "Supergiant Games" {
		t.Errorf("developers = %v", details.DeveloperNames())
	}
}

func TestBackdropURLFallback(t *testing.T) {
	t.Parallel()
	d := &GameDetails{Cover: &Cover{URL: "https://example.com/cover.jpg"}}
	if got := BackdropURL(d); got != "https://example.com/cover.jpg" {
		t.Errorf("BackdropURL() = %q, want cover fallback", got)
	}
}
