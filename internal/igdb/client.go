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

type Client struct {
	clientID     string
	clientSecret string
	baseURL      string
	tokenURL     string
	httpClient   *http.Client

	mu          sync.RWMutex
	accessToken string
	tokenExpiry time.Time
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

func (c *Client) Search(query string, limit int) ([]SearchResult, error) {
	if err := c.ensureToken(); err != nil {
		return nil, err
	}

	body := fmt.Sprintf(
		`fields name,cover.url,first_release_date,platforms.name,category; search "%s"; limit %d;`,
		strings.ReplaceAll(query, `"`, `\"`),
		limit,
	)

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/games", strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("igdb: build request: %w", err)
	}

	c.mu.RLock()
	token := c.accessToken
	c.mu.RUnlock()

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Client-ID", c.clientID)
	req.Header.Set("Content-Type", "text/plain")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("igdb: search request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		// Token may have been revoked; clear it
		c.mu.Lock()
		c.accessToken = ""
		c.mu.Unlock()
		return nil, fmt.Errorf("igdb: unauthorized — token invalidated, retry")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("igdb: search returned %d", resp.StatusCode)
	}

	var results []SearchResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("igdb: decode search results: %w", err)
	}

	// Post-process: normalize cover URLs
	for i := range results {
		if results[i].Cover != nil {
			results[i].Cover.URL = normalizeCoverURL(results[i].Cover.URL)
		}
	}

	return results, nil
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
