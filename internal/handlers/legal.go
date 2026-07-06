package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func PrivacyPage(c *gin.Context) {
	c.HTML(http.StatusOK, "legal/privacy", ViewData(c, nil))
}
