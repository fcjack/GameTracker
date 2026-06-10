package steam

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const storeAPIBase = "https://store.steampowered.com"

// StoreClient resolves Steam store metadata for owned apps.
type StoreClient struct {
	baseURL     string
	httpClient  *http.Client
	rateMu      sync.Mutex
	lastRequest time.Time
	minInterval time.Duration
}

func NewStoreClient() *StoreClient {
	return NewStoreClientWithHTTP(storeAPIBase, nil)
}

func NewStoreClientWithHTTP(baseURL string, httpClient *http.Client) *StoreClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &StoreClient{
		baseURL:     baseURL,
		httpClient:  httpClient,
		minInterval: 350 * time.Millisecond,
	}
}

func (c *StoreClient) SetMinInterval(d time.Duration) {
	c.minInterval = d
}

// IsImportableAppType reports whether a Steam store app should be tracked as a game.
func IsImportableAppType(typ string) bool {
	return typ == "game"
}

func (c *StoreClient) waitRateLimit() {
	if c.minInterval <= 0 {
		return
	}

	c.rateMu.Lock()
	defer c.rateMu.Unlock()

	if elapsed := time.Since(c.lastRequest); elapsed < c.minInterval {
		time.Sleep(c.minInterval - elapsed)
	}
	c.lastRequest = time.Now()
}

// GetAppType returns the Steam store type for an app ID.
// The second value is false when the store has no details for the app.
func (c *StoreClient) GetAppType(ctx context.Context, appID int) (string, bool, error) {
	c.waitRateLimit()

	url := fmt.Sprintf("%s/api/appdetails?appids=%d&filters=basic", c.baseURL, appID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", false, err
	}
	req.Header.Set("User-Agent", "HeliosGameTracker/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("steam store: app %d request: %w", appID, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false, fmt.Errorf("steam store: app %d read response: %w", appID, err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("steam store: app %d returned %d: %s", appID, resp.StatusCode, string(body))
	}

	var result map[string]struct {
		Success bool `json:"success"`
		Data    struct {
			Type string `json:"type"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", false, fmt.Errorf("steam store: app %d decode response: %w", appID, err)
	}

	entry, ok := result[strconv.Itoa(appID)]
	if !ok || !entry.Success {
		return "", false, nil
	}
	return entry.Data.Type, true, nil
}

// FilterImportableGames keeps only Steam store entries classified as games.
func (c *StoreClient) FilterImportableGames(ctx context.Context, games []OwnedGame) ([]OwnedGame, error) {
	if len(games) == 0 {
		return games, nil
	}

	filtered := make([]OwnedGame, 0, len(games))
	for _, g := range games {
		typ, ok, err := c.GetAppType(ctx, g.AppID)
		if err != nil {
			return nil, err
		}
		if ok && IsImportableAppType(typ) {
			filtered = append(filtered, g)
		}
	}
	return filtered, nil
}
