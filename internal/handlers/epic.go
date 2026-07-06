package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jacksoncoelho/game-tracker/internal/crypto"
	"github.com/jacksoncoelho/game-tracker/internal/epic"
	"github.com/jacksoncoelho/game-tracker/internal/models"
)

const epicOAuthStateKey = "epic_oauth_state"

type EpicHandler struct {
	db            *pgxpool.Pool
	client        *epic.Client
	encrypter     *crypto.Encrypter
	importService epicImportStarter
}

type epicImportStarter interface {
	StartEpicImport(ctx context.Context, userID int64) (*models.ImportJob, error)
}

func NewEpicHandler(db *pgxpool.Pool, encrypter *crypto.Encrypter, importService epicImportStarter) *EpicHandler {
	return &EpicHandler{
		db:            db,
		client:        epic.NewClient(os.Getenv("EPIC_CLIENT_ID"), os.Getenv("EPIC_CLIENT_SECRET")),
		encrypter:     encrypter,
		importService: importService,
	}
}

func (h *EpicHandler) Initiate(c *gin.Context) {
	if !h.client.Configured() {
		c.Redirect(http.StatusFound, "/profile?error=error.epic_not_configured")
		return
	}

	state, err := epicRandomState()
	if err != nil {
		c.Redirect(http.StatusFound, "/profile?error=error.epic_auth_failed")
		return
	}

	session := sessions.Default(c)
	session.Set(epicOAuthStateKey, state)
	if err := session.Save(); err != nil {
		c.Redirect(http.StatusFound, "/profile?error=error.epic_auth_failed")
		return
	}

	redirectURI := epicCallbackURL(c)
	c.Redirect(http.StatusFound, h.client.AuthorizeURL(redirectURI, state))
}

func (h *EpicHandler) Callback(c *gin.Context) {
	if oauthErr := c.Query("error"); oauthErr != "" {
		c.Redirect(http.StatusFound, "/profile?error=error.epic_auth_denied")
		return
	}

	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)

	expectedState, _ := session.Get(epicOAuthStateKey).(string)
	session.Delete(epicOAuthStateKey)
	_ = session.Save()

	if expectedState == "" || c.Query("state") != expectedState {
		c.Redirect(http.StatusFound, "/profile?error=error.epic_auth_failed")
		return
	}

	code := c.Query("code")
	if code == "" {
		c.Redirect(http.StatusFound, "/profile?error=error.epic_auth_failed")
		return
	}

	redirectURI := epicCallbackURL(c)
	tokens, err := h.client.ExchangeCode(c.Request.Context(), code, redirectURI)
	if err != nil {
		c.Redirect(http.StatusFound, "/profile?error=error.epic_auth_failed")
		return
	}

	identity, err := h.client.ResolveIdentity(c.Request.Context(), tokens.AccessToken)
	if err != nil {
		if tokens.AccountID != "" {
			identity = &epic.Identity{
				AccountID:   tokens.AccountID,
				DisplayName: tokens.AccountID,
			}
		} else {
			c.Redirect(http.StatusFound, "/profile?error=error.epic_auth_failed")
			return
		}
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
		"epic",
		identity.AccountID,
		identity.DisplayName,
		accessEnc,
		refreshEnc,
		&expiresAt,
	)
	if err != nil {
		c.Redirect(http.StatusFound, "/profile?error=error.link_account_failed")
		return
	}

	if h.importService != nil {
		_, _ = h.importService.StartEpicImport(c.Request.Context(), userID)
	}

	c.Redirect(http.StatusFound, "/profile")
}

func epicCallbackURL(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil || c.Request.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/auth/epic/callback", scheme, c.Request.Host)
}

func epicRandomState() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
