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

func (h *ImportHandler) UnlinkSteamAccount(c *gin.Context) {
	h.unlinkAccount(c, "steam")
}

func (h *ImportHandler) UnlinkXboxAccount(c *gin.Context) {
	h.unlinkAccount(c, "xbox")
}

func (h *ImportHandler) unlinkAccount(c *gin.Context, provider string) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)

	fail := func(errorKey string) {
		c.Redirect(http.StatusSeeOther, "/profile?error="+errorKey)
	}

	active, err := models.HasActiveImportJob(c.Request.Context(), h.db, userID, provider)
	if err != nil {
		fail(unlinkFailedErrorKey(provider))
		return
	}
	if active {
		fail("error.import_in_progress")
		return
	}

	_, err = models.GetLinkedAccount(c.Request.Context(), h.db, userID, provider)
	if err != nil {
		if err == pgx.ErrNoRows {
			fail(notLinkedErrorKey(provider))
			return
		}
		fail(unlinkFailedErrorKey(provider))
		return
	}

	switch provider {
	case "steam":
		if _, err = models.RemoveSteamGamesFromLibrary(c.Request.Context(), h.db, userID); err != nil {
			fail("error.unlink_steam_failed")
			return
		}
	case "xbox":
		if _, err = models.RemoveXboxGamesFromLibrary(c.Request.Context(), h.db, userID); err != nil {
			fail("error.unlink_xbox_failed")
			return
		}
	}

	if err := models.DeleteLinkedAccount(c.Request.Context(), h.db, userID, provider); err != nil {
		fail(unlinkFailedErrorKey(provider))
		return
	}

	c.Redirect(http.StatusSeeOther, fmt.Sprintf("/profile?%s_unlinked=1", provider))
}

func notLinkedErrorKey(provider string) string {
	if provider == "xbox" {
		return "error.xbox_not_linked"
	}
	return "error.steam_not_linked"
}

func unlinkFailedErrorKey(provider string) string {
	if provider == "xbox" {
		return "error.unlink_xbox_failed"
	}
	return "error.unlink_steam_failed"
}

func (h *ImportHandler) CancelSteamImport(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)
	locale := LocaleFromContext(c)

	_, err := models.GetLinkedAccount(c.Request.Context(), h.db, userID, "steam")
	if err != nil {
		if err == pgx.ErrNoRows {
			c.Redirect(http.StatusSeeOther, "/profile?error=error.steam_not_linked")
			return
		}
		c.Redirect(http.StatusSeeOther, "/profile?error=error.cancel_import_failed")
		return
	}

	_, err = h.service.CancelImport(c.Request.Context(), userID, "steam", i18n.T(locale, "import.cancelled"))
	if err != nil {
		c.Redirect(http.StatusSeeOther, "/profile?error=error.cancel_import_failed")
		return
	}

	c.Redirect(http.StatusSeeOther, "/profile")
}

func (h *ImportHandler) XboxImportStatus(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)
	locale := LocaleFromContext(c)

	job, err := models.GetLatestImportJob(c.Request.Context(), h.db, userID, "xbox")
	if err != nil {
		if err == pgx.ErrNoRows {
			c.HTML(http.StatusOK, "profile/xbox_import_status", ViewData(c, gin.H{}))
			return
		}
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.HTML(http.StatusOK, "profile/xbox_import_status", ViewData(c, gin.H{
		"xboxImportJob":        job,
		"xboxImportJobSummary": i18n.ImportJobSummary(job, locale),
	}))
}

func (h *ImportHandler) StartXboxImport(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)

	_, err := models.GetLinkedAccount(c.Request.Context(), h.db, userID, "xbox")
	if err != nil {
		if err == pgx.ErrNoRows {
			c.Redirect(http.StatusSeeOther, "/profile?error=error.xbox_not_linked")
			return
		}
		c.Redirect(http.StatusSeeOther, "/profile?error=error.start_import_failed")
		return
	}

	_, err = h.service.StartXboxImport(c.Request.Context(), userID)
	if err != nil {
		c.Redirect(http.StatusSeeOther, "/profile?error=error.start_import_failed")
		return
	}

	c.Redirect(http.StatusSeeOther, "/profile")
}

func (h *ImportHandler) ClearXboxLibrary(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)

	active, err := models.HasActiveImportJob(c.Request.Context(), h.db, userID, "xbox")
	if err != nil {
		c.Redirect(http.StatusSeeOther, "/profile?error=error.clear_xbox_library_failed")
		return
	}
	if active {
		c.Redirect(http.StatusSeeOther, "/profile?error=error.import_in_progress")
		return
	}

	_, err = models.GetLinkedAccount(c.Request.Context(), h.db, userID, "xbox")
	if err != nil {
		if err == pgx.ErrNoRows {
			c.Redirect(http.StatusSeeOther, "/profile?error=error.xbox_not_linked")
			return
		}
		c.Redirect(http.StatusSeeOther, "/profile?error=error.clear_xbox_library_failed")
		return
	}

	removed, err := models.RemoveXboxGamesFromLibrary(c.Request.Context(), h.db, userID)
	if err != nil {
		c.Redirect(http.StatusSeeOther, "/profile?error=error.clear_xbox_library_failed")
		return
	}

	c.Redirect(http.StatusSeeOther, fmt.Sprintf("/profile?xbox_cleared=%d", removed))
}

func (h *ImportHandler) CancelXboxImport(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)
	locale := LocaleFromContext(c)

	_, err := models.GetLinkedAccount(c.Request.Context(), h.db, userID, "xbox")
	if err != nil {
		if err == pgx.ErrNoRows {
			c.Redirect(http.StatusSeeOther, "/profile?error=error.xbox_not_linked")
			return
		}
		c.Redirect(http.StatusSeeOther, "/profile?error=error.cancel_import_failed")
		return
	}

	_, err = h.service.CancelImport(c.Request.Context(), userID, "xbox", i18n.T(locale, "import.cancelled"))
	if err != nil {
		c.Redirect(http.StatusSeeOther, "/profile?error=error.cancel_import_failed")
		return
	}

	c.Redirect(http.StatusSeeOther, "/profile")
}
