package epic

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultAuthorizeURL = "https://www.epicgames.com/id/authorize"
	defaultTokenURL     = "https://api.epicgames.dev/epic/oauth/v2/token"
	defaultUserInfoURL  = "https://api.epicgames.dev/epic/oauth/v2/userInfo"
	defaultScope        = "basic_profile"
)

type Client struct {
	clientID     string
	clientSecret string
	authorizeURL string
	tokenURL     string
	userInfoURL  string
	httpClient   *http.Client
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	AccountID    string
}

type Identity struct {
	AccountID   string
	DisplayName string
}

func NewClient(clientID, clientSecret string) *Client {
	return NewClientWithHTTP(clientID, clientSecret, nil)
}

func NewClientWithHTTP(clientID, clientSecret string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		clientID:     clientID,
		clientSecret: clientSecret,
		authorizeURL: defaultAuthorizeURL,
		tokenURL:     defaultTokenURL,
		userInfoURL:  defaultUserInfoURL,
		httpClient:   httpClient,
	}
}

// SetEndpoints overrides API URLs. Used by tests and custom deployments.
func (c *Client) SetEndpoints(tokenURL, userInfoURL string) {
	if tokenURL != "" {
		c.tokenURL = tokenURL
	}
	if userInfoURL != "" {
		c.userInfoURL = userInfoURL
	}
}

func (c *Client) Configured() bool {
	return c.clientID != "" && c.clientSecret != ""
}

func (c *Client) AuthorizeURL(redirectURI, state string) string {
	params := url.Values{}
	params.Set("client_id", c.clientID)
	params.Set("response_type", "code")
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", defaultScope)
	params.Set("state", state)
	return c.authorizeURL + "?" + params.Encode()
}

func (c *Client) ExchangeCode(ctx context.Context, code, redirectURI string) (*TokenPair, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	return c.requestToken(ctx, form)
}

func (c *Client) RefreshToken(ctx context.Context, refreshToken string) (*TokenPair, error) {
	if refreshToken == "" {
		return nil, fmt.Errorf("epic: refresh token is required")
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)

	pair, err := c.requestToken(ctx, form)
	if err != nil {
		return nil, err
	}
	if pair.RefreshToken == "" {
		pair.RefreshToken = refreshToken
	}
	return pair, nil
}

func (c *Client) requestToken(ctx context.Context, form url.Values) (*TokenPair, error) {
	if !c.Configured() {
		return nil, fmt.Errorf("epic: client credentials are not configured")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("epic: create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic "+basicAuth(c.clientID, c.clientSecret))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("epic: token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("epic: read token response: %w", err)
	}

	return parseTokenResponse(body)
}

func parseTokenResponse(body []byte) (*TokenPair, error) {
	var parsed struct {
		AccessToken      string `json:"access_token"`
		RefreshToken     string `json:"refresh_token"`
		ExpiresIn        int    `json:"expires_in"`
		AccountID        string `json:"account_id"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("epic: decode token response: %w", err)
	}
	if parsed.Error != "" {
		desc := strings.TrimSpace(parsed.ErrorDescription)
		if desc == "" {
			desc = parsed.Error
		}
		return nil, fmt.Errorf("epic: token request failed: %s", desc)
	}
	if parsed.AccessToken == "" {
		return nil, fmt.Errorf("epic: token response missing access token")
	}

	expiresAt := time.Now().Add(time.Duration(parsed.ExpiresIn) * time.Second)
	if parsed.ExpiresIn <= 0 {
		expiresAt = time.Now().Add(2 * time.Hour)
	}

	return &TokenPair{
		AccessToken:  parsed.AccessToken,
		RefreshToken: parsed.RefreshToken,
		ExpiresAt:    expiresAt,
		AccountID:    parsed.AccountID,
	}, nil
}

func (c *Client) ResolveIdentity(ctx context.Context, accessToken string) (*Identity, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.userInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("epic: create userInfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("epic: userInfo request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("epic: read userInfo response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("epic: userInfo returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed struct {
		Sub               string `json:"sub"`
		AccountID         string `json:"account_id"`
		PreferredUsername string `json:"preferred_username"`
		Name              string `json:"name"`
		DisplayName       string `json:"displayName"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("epic: decode userInfo response: %w", err)
	}

	accountID := parsed.AccountID
	if accountID == "" {
		accountID = parsed.Sub
	}
	if accountID == "" {
		return nil, fmt.Errorf("epic: userInfo missing account id")
	}

	displayName := parsed.DisplayName
	if displayName == "" {
		displayName = parsed.PreferredUsername
	}
	if displayName == "" {
		displayName = parsed.Name
	}
	if displayName == "" {
		displayName = accountID
	}

	return &Identity{
		AccountID:   accountID,
		DisplayName: displayName,
	}, nil
}

func basicAuth(clientID, clientSecret string) string {
	return base64.StdEncoding.EncodeToString([]byte(clientID + ":" + clientSecret))
}
