package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jacksoncoelho/game-tracker/internal/i18n"
	"github.com/jacksoncoelho/game-tracker/internal/importjob"
	"github.com/jacksoncoelho/game-tracker/internal/models"
)

type ImportHandler struct {
	db      *pgxpool.Pool
	service *importjob.Service
}

func NewImportHandler(db *pgxpool.Pool, service *importjob.Service) *ImportHandler {
	return &ImportHandler{db: db, service: service}
}

func (h *ImportHandler) SteamImportStatus(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)
	locale := LocaleFromContext(c)

	job, err := models.GetLatestImportJob(c.Request.Context(), h.db, userID, "steam")
	if err != nil {
		if err == pgx.ErrNoRows {
			c.HTML(http.StatusOK, "profile/steam_import_status", ViewData(c, gin.H{}))
			return
		}
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.HTML(http.StatusOK, "profile/steam_import_status", ViewData(c, gin.H{
		"steamImportJob":   job,
		"importJobSummary": i18n.ImportJobSummary(job, locale),
	}))
}

func (h *ImportHandler) StartSteamImport(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)

	account, err := models.GetLinkedAccount(c.Request.Context(), h.db, userID, "steam")
	if err != nil {
		if err == pgx.ErrNoRows {
			c.Redirect(http.StatusSeeOther, "/profile?error=error.steam_not_linked")
			return
		}
		c.Redirect(http.StatusSeeOther, "/profile?error=error.start_import_failed")
		return
	}

	_, err = h.service.StartSteamImport(c.Request.Context(), userID, account.ExternalID)
	if err != nil {
		c.Redirect(http.StatusSeeOther, "/profile?error=error.start_import_failed")
		return
	}

	c.Redirect(http.StatusSeeOther, "/profile")
}

func (h *ImportHandler) ClearSteamLibrary(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)

	active, err := models.HasActiveImportJob(c.Request.Context(), h.db, userID, "steam")
	if err != nil {
		c.Redirect(http.StatusSeeOther, "/profile?error=error.clear_steam_library_failed")
		return
	}
	if active {
		c.Redirect(http.StatusSeeOther, "/profile?error=error.import_in_progress")
		return
	}

	_, err = models.GetLinkedAccount(c.Request.Context(), h.db, userID, "steam")
	if err != nil {
		if err == pgx.ErrNoRows {
			c.Redirect(http.StatusSeeOther, "/profile?error=error.steam_not_linked")
			return
		}
		c.Redirect(http.StatusSeeOther, "/profile?error=error.clear_steam_library_failed")
		return
	}

	removed, err := models.RemoveSteamGamesFromLibrary(c.Request.Context(), h.db, userID)
	if err != nil {
		c.Redirect(http.StatusSeeOther, "/profile?error=error.clear_steam_library_failed")
		return
	}

	c.Redirect(http.StatusSeeOther, fmt.Sprintf("/profile?steam_cleared=%d", removed))
}
