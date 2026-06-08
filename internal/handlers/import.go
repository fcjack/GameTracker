package handlers

import (
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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

	job, err := models.GetLatestImportJob(c.Request.Context(), h.db, userID, "steam")
	if err != nil {
		if err == pgx.ErrNoRows {
			c.HTML(http.StatusOK, "profile/steam_import_status", gin.H{})
			return
		}
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	c.HTML(http.StatusOK, "profile/steam_import_status", gin.H{
		"steamImportJob": job,
	})
}

func (h *ImportHandler) StartSteamImport(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)

	account, err := models.GetLinkedAccount(c.Request.Context(), h.db, userID, "steam")
	if err != nil {
		if err == pgx.ErrNoRows {
			c.Redirect(http.StatusSeeOther, "/profile?error=Steam+account+not+linked")
			return
		}
		c.Redirect(http.StatusSeeOther, "/profile?error=Failed+to+start+import")
		return
	}

	_, err = h.service.StartSteamImport(c.Request.Context(), userID, account.ExternalID)
	if err != nil {
		c.Redirect(http.StatusSeeOther, "/profile?error=Failed+to+start+import")
		return
	}

	c.Redirect(http.StatusSeeOther, "/profile")
}
