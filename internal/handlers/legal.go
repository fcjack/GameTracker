package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jacksoncoelho/game-tracker/internal/i18n"
	"github.com/jacksoncoelho/game-tracker/internal/version"
)

// PrivacyPage serves a public privacy policy always rendered in English.
func PrivacyPage(c *gin.Context) {
	c.HTML(http.StatusOK, "legal/privacy", gin.H{
		"lang":    i18n.LocaleEN,
		"T":       i18n.NewTranslator(i18n.LocaleEN),
		"version": version.Version,
	})
}
