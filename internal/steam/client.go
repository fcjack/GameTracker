package steam

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const steamAPIBase = "https://api.steampowered.com"

type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

type OwnedGame struct {
	AppID int    `json:"appid"`
	Name  string `json:"name"`
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: steamAPIBase,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) GetOwnedGames(steamID string) ([]OwnedGame, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("steam: STEAM_API_KEY is not configured")
	}

	url := fmt.Sprintf(
		"%s/IPlayerService/GetOwnedGames/v0001/?key=%s&steamid=%s&include_appinfo=1&include_played_free_games=1&format=json",
		c.baseURL, c.apiKey, steamID,
	)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("steam: owned games request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("steam: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("steam: owned games returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Response struct {
			GameCount int         `json:"game_count"`
			Games     []OwnedGame `json:"games"`
		} `json:"response"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("steam: decode response: %w", err)
	}

	if result.Response.Games == nil {
		return []OwnedGame{}, nil
	}
	return result.Response.Games, nil
}
