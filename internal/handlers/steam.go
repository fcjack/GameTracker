package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jacksoncoelho/game-tracker/internal/models"
)

type SteamHandler struct {
	db          *pgxpool.Pool
	openIDURL   string
	steamAPIURL string
	httpClient  *http.Client
}

func NewSteamHandler(db *pgxpool.Pool) *SteamHandler {
	return &SteamHandler{
		db:          db,
		openIDURL:   "https://steamcommunity.com/openid/login",
		steamAPIURL: "https://api.steampowered.com",
		httpClient:  http.DefaultClient,
	}
}

func (h *SteamHandler) Initiate(c *gin.Context) {
	scheme := "http"
	if c.Request.TLS != nil || c.Request.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	base := fmt.Sprintf("%s://%s", scheme, c.Request.Host)
	returnTo := fmt.Sprintf("%s/auth/steam/callback", base)

	params := url.Values{}
	params.Set("openid.ns", "http://specs.openid.net/auth/2.0")
	params.Set("openid.mode", "checkid_setup")
	params.Set("openid.return_to", returnTo)
	params.Set("openid.realm", base)
	params.Set("openid.identity", "http://specs.openid.net/auth/2.0/identifier_select")
	params.Set("openid.claimed_id", "http://specs.openid.net/auth/2.0/identifier_select")

	redirectURL := "https://steamcommunity.com/openid/login?" + params.Encode()
	c.Redirect(http.StatusFound, redirectURL)
}

func (h *SteamHandler) Callback(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)

	params := make(url.Values)
	for key := range c.Request.URL.Query() {
		if strings.HasPrefix(key, "openid.") {
			params[key] = c.Request.URL.Query()[key]
		}
	}

	params.Set("openid.mode", "check_authentication")

	resp, err := h.httpClient.PostForm(h.openIDURL, params)
	if err != nil {
		c.Redirect(http.StatusFound, "/profile?error=Steam+verification+failed")
		return
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.Redirect(http.StatusFound, "/profile?error=Steam+verification+failed")
		return
	}

	if !strings.Contains(string(body), "is_valid:true") {
		c.Redirect(http.StatusFound, "/profile?error=Steam+verification+failed")
		return
	}

	claimedID := c.Query("openid.claimed_id")
	if claimedID == "" {
		c.Redirect(http.StatusFound, "/profile?error=Invalid+Steam+response")
		return
	}

	steamIDStr := extractSteamID(claimedID)
	if steamIDStr == "" {
		c.Redirect(http.StatusFound, "/profile?error=Invalid+Steam+ID")
		return
	}

	personaName := steamIDStr
	apiKey := os.Getenv("STEAM_API_KEY")
	if apiKey != "" {
		name, err := h.fetchSteamPersonaName(steamIDStr, apiKey)
		if err == nil && name != "" {
			personaName = name
		}
	}

	_, err = models.UpsertLinkedAccount(
		c.Request.Context(),
		h.db,
		userID,
		"steam",
		steamIDStr, // external_id: SteamID64 used to fetch game library via Steam Web API
		personaName,
		"", // Steam OpenID doesn't provide OAuth tokens; we use app's STEAM_API_KEY + SteamID
		"",
		nil,
	)
	if err != nil {
		c.Redirect(http.StatusFound, "/profile?error=Failed+to+link+account")
		return
	}

	c.Redirect(http.StatusFound, "/profile")
}

func extractSteamID(claimedID string) string {
	re := regexp.MustCompile(`/(\d+)$`)
	matches := re.FindStringSubmatch(claimedID)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func (h *SteamHandler) fetchSteamPersonaName(steamID, apiKey string) (string, error) {
	apiURL := fmt.Sprintf(
		"%s/ISteamUser/GetPlayerSummaries/v0002/?key=%s&steamids=%s",
		h.steamAPIURL,
		apiKey,
		steamID,
	)

	resp, err := h.httpClient.Get(apiURL)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		Response struct {
			Players []struct {
				Personaname string `json:"personaname"`
			} `json:"players"`
		} `json:"response"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if len(result.Response.Players) > 0 {
		return result.Response.Players[0].Personaname, nil
	}

	return "", nil
}
