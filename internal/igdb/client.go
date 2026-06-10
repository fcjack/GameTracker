package igdb

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	externalGameCategorySteam = 1
	gameCategoryMainGame      = 0
	// IGDB allows 4 requests/second; stay under with a 300ms minimum gap.
	minRequestInterval = 300 * time.Millisecond
)

// IsMainGame reports whether an IGDB game category is a main game entry.
func IsMainGame(category int) bool {
	return category == gameCategoryMainGame
}

type Client struct {
	clientID     string
	clientSecret string
	baseURL      string
	tokenURL     string
	httpClient   *http.Client

	mu          sync.RWMutex
	accessToken string
	tokenExpiry time.Time

	rateMu       sync.Mutex
	lastRequest  time.Time
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
}

type Platform struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Cover struct {
	URL string `json:"url"`
}

type SearchResult struct {
	ID               int64      `json:"id"`
	Name             string     `json:"name"`
	Category         int        `json:"category"`
	Cover            *Cover     `json:"cover"`
	FirstReleaseDate int64      `json:"first_release_date"`
	Platforms        []Platform `json:"platforms"`
}

func NewClient(clientID, clientSecret, baseURL string) *Client {
	return &Client{
		clientID:     clientID,
		clientSecret: clientSecret,
		baseURL:      baseURL,
		tokenURL:     "https://id.twitch.tv/oauth2/token",
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) SetTokenURL(url string) {
	c.tokenURL = url
}

func (c *Client) SetHTTPClient(client *http.Client) {
	c.httpClient = client
}

func (c *Client) ensureToken() error {
	c.mu.RLock()
	valid := c.accessToken != "" && time.Now().Before(c.tokenExpiry)
	c.mu.RUnlock()
	if valid {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	// Double-check after acquiring write lock
	if c.accessToken != "" && time.Now().Before(c.tokenExpiry) {
		return nil
	}

	resp, err := c.httpClient.PostForm(c.tokenURL, url.Values{
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
		"grant_type":    {"client_credentials"},
	})
	if err != nil {
		return fmt.Errorf("igdb: token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("igdb: token request returned %d", resp.StatusCode)
	}

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return fmt.Errorf("igdb: decode token response: %w", err)
	}

	c.accessToken = tr.AccessToken
	// Subtract 5 minutes as a safety buffer before the real expiry
	c.tokenExpiry = time.Now().Add(time.Duration(tr.ExpiresIn-300) * time.Second)
	return nil
}

func (c *Client) waitForRateLimit() {
	c.rateMu.Lock()
	defer c.rateMu.Unlock()

	if elapsed := time.Since(c.lastRequest); elapsed < minRequestInterval {
		time.Sleep(minRequestInterval - elapsed)
	}
	c.lastRequest = time.Now()
}

func (c *Client) post(endpoint, body string, dest any) error {
	if err := c.ensureToken(); err != nil {
		return err
	}

	c.waitForRateLimit()

	req, err := http.NewRequest(http.MethodPost, c.baseURL+endpoint, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("igdb: build request: %w", err)
	}

	c.mu.RLock()
	token := c.accessToken
	c.mu.RUnlock()

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Client-ID", c.clientID)
	req.Header.Set("Content-Type", "text/plain")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("igdb: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		c.mu.Lock()
		c.accessToken = ""
		c.mu.Unlock()
		return fmt.Errorf("igdb: unauthorized — token invalidated, retry")
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		time.Sleep(2 * time.Second)
		return fmt.Errorf("igdb: rate limited")
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("igdb: request returned %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("igdb: decode response: %w", err)
	}
	return nil
}

func (c *Client) Search(query string, limit int) ([]SearchResult, error) {
	body := fmt.Sprintf(
		`fields name,cover.url,first_release_date,platforms.name,category; search "%s"; limit %d;`,
		strings.ReplaceAll(query, `"`, `\"`),
		limit,
	)

	var results []SearchResult
	if err := c.post("/games", body, &results); err != nil {
		return nil, err
	}

	for i := range results {
		if results[i].Cover != nil {
			results[i].Cover.URL = normalizeCoverURL(results[i].Cover.URL)
		}
	}

	return results, nil
}

type externalGameResult struct {
	Game     int64 `json:"game"`
	Category int   `json:"category"`
}

// LookupIGDBIDBySteamAppID resolves a Steam app ID to an IGDB game.
// Steam name disambiguates when the same uid exists across multiple storefronts.
func (c *Client) LookupIGDBIDBySteamAppID(appID int, steamName string) (int64, error) {
	if steamName != "" {
		body := fmt.Sprintf(
			`fields name; where external_games.uid = "%d" & name = "%s"; limit 1;`,
			appID,
			strings.ReplaceAll(steamName, `"`, `\"`),
		)
		var results []SearchResult
		if err := c.post("/games", body, &results); err != nil {
			return 0, err
		}
		if len(results) > 0 {
			return results[0].ID, nil
		}
	}

	body := fmt.Sprintf(`fields game,category; where uid = "%d"; limit 10;`, appID)
	var results []externalGameResult
	if err := c.post("/external_games", body, &results); err != nil {
		return 0, err
	}

	var fallback int64
	for _, r := range results {
		if r.Game == 0 {
			continue
		}
		switch r.Category {
		case 0, externalGameCategorySteam:
			if fallback == 0 {
				fallback = r.Game
			}
		}
	}
	return fallback, nil
}

func (c *Client) GetGameByID(id int64) (*SearchResult, error) {
	body := fmt.Sprintf(
		`fields name,cover.url,first_release_date,platforms.name,category; where id = %d; limit 1;`,
		id,
	)

	var results []SearchResult
	if err := c.post("/games", body, &results); err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}

	if results[0].Cover != nil {
		results[0].Cover.URL = normalizeCoverURL(results[0].Cover.URL)
	}
	return &results[0], nil
}

func normalizeCoverURL(raw string) string {
	if raw == "" {
		return ""
	}
	url := strings.ReplaceAll(raw, "t_thumb", "t_cover_big")
	if strings.HasPrefix(url, "//") {
		url = "https:" + url
	}
	return url
}

func ReleaseYear(unixTimestamp int64) int {
	if unixTimestamp == 0 {
		return 0
	}
	return time.Unix(unixTimestamp, 0).Year()
}
