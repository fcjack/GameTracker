package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jacksoncoelho/game-tracker/internal/crypto"
	"github.com/jacksoncoelho/game-tracker/internal/models"
	"github.com/jacksoncoelho/game-tracker/internal/xbox"
)

const xboxOAuthStateKey = "xbox_oauth_state"

type XboxHandler struct {
	db        *pgxpool.Pool
	client    *xbox.Client
	encrypter *crypto.Encrypter
}

func NewXboxHandler(db *pgxpool.Pool, encrypter *crypto.Encrypter) *XboxHandler {
	return &XboxHandler{
		db:        db,
		client:    xbox.NewClient(os.Getenv("XBOX_CLIENT_ID"), os.Getenv("XBOX_CLIENT_SECRET")),
		encrypter: encrypter,
	}
}

func (h *XboxHandler) Initiate(c *gin.Context) {
	if !h.client.Configured() {
		c.Redirect(http.StatusFound, "/profile?error=error.xbox_not_configured")
		return
	}

	state, err := randomState()
	if err != nil {
		c.Redirect(http.StatusFound, "/profile?error=error.xbox_auth_failed")
		return
	}

	session := sessions.Default(c)
	session.Set(xboxOAuthStateKey, state)
	if err := session.Save(); err != nil {
		c.Redirect(http.StatusFound, "/profile?error=error.xbox_auth_failed")
		return
	}

	redirectURI := xboxCallbackURL(c)
	c.Redirect(http.StatusFound, h.client.AuthorizeURL(redirectURI, state))
}

func (h *XboxHandler) Callback(c *gin.Context) {
	if oauthErr := c.Query("error"); oauthErr != "" {
		c.Redirect(http.StatusFound, "/profile?error=error.xbox_auth_denied")
		return
	}

	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)

	expectedState, _ := session.Get(xboxOAuthStateKey).(string)
	session.Delete(xboxOAuthStateKey)
	_ = session.Save()

	if expectedState == "" || c.Query("state") != expectedState {
		c.Redirect(http.StatusFound, "/profile?error=error.xbox_auth_failed")
		return
	}

	code := c.Query("code")
	if code == "" {
		c.Redirect(http.StatusFound, "/profile?error=error.xbox_auth_failed")
		return
	}

	redirectURI := xboxCallbackURL(c)
	tokens, err := h.client.ExchangeCode(c.Request.Context(), code, redirectURI)
	if err != nil {
		c.Redirect(http.StatusFound, "/profile?error=error.xbox_auth_failed")
		return
	}

	identity, err := h.client.ResolveIdentity(c.Request.Context(), tokens.AccessToken)
	if err != nil {
		c.Redirect(http.StatusFound, "/profile?error=error.xbox_auth_failed")
		return
	}

	accessEnc, err := h.encrypter.Encrypt(tokens.AccessToken)
	if err != nil {
		c.Redirect(http.StatusFound, "/profile?error=error.link_account_failed")
		return
	}

	refreshEnc := ""
	if tokens.RefreshToken != "" {
		refreshEnc, err = h.encrypter.Encrypt(tokens.RefreshToken)
		if err != nil {
			c.Redirect(http.StatusFound, "/profile?error=error.link_account_failed")
			return
		}
	}

	expiresAt := tokens.ExpiresAt
	_, err = models.UpsertLinkedAccount(
		c.Request.Context(),
		h.db,
		userID,
		"xbox",
		identity.XUID,
		identity.Gamertag,
		accessEnc,
		refreshEnc,
		&expiresAt,
	)
	if err != nil {
		c.Redirect(http.StatusFound, "/profile?error=error.link_account_failed")
		return
	}

	c.Redirect(http.StatusFound, "/profile")
}

func xboxCallbackURL(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil || c.Request.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/auth/xbox/callback", scheme, c.Request.Host)
}

func randomState() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
