package xbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	defaultAuthorizeURL = "https://login.microsoftonline.com/consumers/oauth2/v2.0/authorize"
	defaultTokenURL     = "https://login.microsoftonline.com/consumers/oauth2/v2.0/token"
	defaultUserAuthURL  = "https://user.auth.xboxlive.com/user/authenticate"
	defaultXSTSURL      = "https://xsts.auth.xboxlive.com/xsts/authorize"
	defaultTitleHubURL  = "https://titlehub.xboxlive.com"
	defaultUserStatsURL = "https://userstats.xboxlive.com"
	defaultScope        = "xboxlive.signin xboxlive.offline_access"
)

type Client struct {
	clientID     string
	clientSecret string
	authorizeURL string
	tokenURL     string
	userAuthURL  string
	xstsURL      string
	titleHubURL  string
	userStatsURL string
	httpClient   *http.Client
	scidCache    sync.Map
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

type Identity struct {
	XUID     string
	Gamertag string
}

// XSTSSession holds the authenticated Xbox Live session used for service calls.
type XSTSSession struct {
	Token    string
	UserHash string
	XUID     string
	Gamertag string
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
		userAuthURL:  defaultUserAuthURL,
		xstsURL:      defaultXSTSURL,
		titleHubURL:  defaultTitleHubURL,
		userStatsURL: defaultUserStatsURL,
		httpClient:   httpClient,
	}
}

// SetEndpoints overrides API URLs. Used by tests and custom deployments.
func (c *Client) SetEndpoints(tokenURL, userAuthURL, xstsURL string) {
	if tokenURL != "" {
		c.tokenURL = tokenURL
	}
	if userAuthURL != "" {
		c.userAuthURL = userAuthURL
	}
	if xstsURL != "" {
		c.xstsURL = xstsURL
	}
}

// SetTitleHubURL overrides the Title Hub base URL. Used by tests.
func (c *Client) SetTitleHubURL(titleHubURL string) {
	if titleHubURL != "" {
		c.titleHubURL = titleHubURL
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
		return nil, fmt.Errorf("xbox: refresh token is required")
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("scope", defaultScope)

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
		return nil, fmt.Errorf("xbox: client credentials are not configured")
	}

	form.Set("client_id", c.clientID)
	form.Set("client_secret", c.clientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("xbox: create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("xbox: token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("xbox: read token response: %w", err)
	}

	return parseTokenResponse(body)
}

func parseTokenResponse(body []byte) (*TokenPair, error) {
	var parsed struct {
		AccessToken      string `json:"access_token"`
		RefreshToken     string `json:"refresh_token"`
		ExpiresIn        int    `json:"expires_in"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("xbox: decode token response: %w", err)
	}
	if parsed.Error != "" {
		desc := strings.TrimSpace(parsed.ErrorDescription)
		if desc == "" {
			desc = parsed.Error
		}
		return nil, fmt.Errorf("xbox: token request failed: %s", desc)
	}
	if parsed.AccessToken == "" {
		return nil, fmt.Errorf("xbox: token response missing access token")
	}

	expiresAt := time.Now().Add(time.Duration(parsed.ExpiresIn) * time.Second)
	if parsed.ExpiresIn <= 0 {
		expiresAt = time.Now().Add(time.Hour)
	}

	return &TokenPair{
		AccessToken:  parsed.AccessToken,
		RefreshToken: parsed.RefreshToken,
		ExpiresAt:    expiresAt,
	}, nil
}

func (c *Client) Authenticate(ctx context.Context, accessToken string) (*XSTSSession, error) {
	userToken, err := c.authenticateUser(ctx, accessToken)
	if err != nil {
		return nil, err
	}

	xsts, err := c.authorizeXSTS(ctx, userToken)
	if err != nil {
		return nil, err
	}

	if xsts.Token == "" {
		return nil, fmt.Errorf("xbox: XSTS response missing token")
	}
	if len(xsts.DisplayClaims.Xui) == 0 {
		return nil, fmt.Errorf("xbox: missing identity claims")
	}

	claim := xsts.DisplayClaims.Xui[0]
	if claim.Xid == "" {
		return nil, fmt.Errorf("xbox: missing XUID")
	}
	if claim.Uhs == "" {
		return nil, fmt.Errorf("xbox: missing user hash")
	}

	gamertag := claim.Gtg
	if gamertag == "" {
		gamertag = claim.Xid
	}

	return &XSTSSession{
		Token:    xsts.Token,
		UserHash: claim.Uhs,
		XUID:     claim.Xid,
		Gamertag: gamertag,
	}, nil
}

func (c *Client) ResolveIdentity(ctx context.Context, accessToken string) (*Identity, error) {
	session, err := c.Authenticate(ctx, accessToken)
	if err != nil {
		return nil, err
	}

	return &Identity{
		XUID:     session.XUID,
		Gamertag: session.Gamertag,
	}, nil
}

func (c *Client) authenticateUser(ctx context.Context, accessToken string) (string, error) {
	payload := map[string]any{
		"RelyingParty": "http://auth.xboxlive.com",
		"TokenType":    "JWT",
		"Properties": map[string]string{
			"AuthMethod": "RPS",
			"SiteName":   "user.auth.xboxlive.com",
			"RpsTicket":  "d=" + accessToken,
		},
	}

	var parsed struct {
		Token string `json:"Token"`
	}
	if err := c.postJSON(ctx, c.userAuthURL, payload, &parsed, "1"); err != nil {
		return "", err
	}
	if parsed.Token == "" {
		return "", fmt.Errorf("xbox: user authenticate response missing token")
	}
	return parsed.Token, nil
}

func (c *Client) authorizeXSTS(ctx context.Context, userToken string) (*xstsResponse, error) {
	payload := map[string]any{
		"RelyingParty": "http://xboxlive.com",
		"TokenType":    "JWT",
		"Properties": map[string]any{
			"SandboxId":  "RETAIL",
			"UserTokens": []string{userToken},
		},
	}

	var parsed xstsResponse
	if err := c.postJSON(ctx, c.xstsURL, payload, &parsed, "1"); err != nil {
		return nil, err
	}
	return &parsed, nil
}

type xstsResponse struct {
	Token         string `json:"Token"`
	DisplayClaims struct {
		Xui []struct {
			Uhs string `json:"uhs"`
			Xid string `json:"xid"`
			Gtg string `json:"gtg"`
		} `json:"xui"`
	} `json:"DisplayClaims"`
}

func (c *Client) postXBLJSON(ctx context.Context, endpoint string, session *XSTSSession, payload any, dest any, contractVersion string) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("xbox: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("xbox: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en-US")
	req.Header.Set("Authorization", fmt.Sprintf("XBL3.0 x=%s;%s", session.UserHash, session.Token))
	req.Header.Set("x-xbl-contract-version", contractVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("xbox: request %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("xbox: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("xbox: %s returned %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	if err := json.Unmarshal(respBody, dest); err != nil {
		return fmt.Errorf("xbox: decode response: %w", err)
	}
	return nil
}

func (c *Client) getXBLJSON(ctx context.Context, endpoint string, session *XSTSSession, contractVersion string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("xbox: create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en-US")
	req.Header.Set("Authorization", fmt.Sprintf("XBL3.0 x=%s;%s", session.UserHash, session.Token))
	req.Header.Set("x-xbl-contract-version", contractVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("xbox: request %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("xbox: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("xbox: %s returned %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	if err := json.Unmarshal(respBody, dest); err != nil {
		return fmt.Errorf("xbox: decode response: %w", err)
	}
	return nil
}

func (c *Client) postJSON(ctx context.Context, endpoint string, payload any, dest any, contractVersion string) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("xbox: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("xbox: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-xbl-contract-version", contractVersion)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("xbox: request %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("xbox: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("xbox: %s returned %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	if err := json.Unmarshal(respBody, dest); err != nil {
		return fmt.Errorf("xbox: decode response: %w", err)
	}
	return nil
}
