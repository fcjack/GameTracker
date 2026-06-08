package handlers

import (
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jacksoncoelho/game-tracker/internal/models"
)

type ProfileHandler struct {
	db *pgxpool.Pool
}

func NewProfileHandler(db *pgxpool.Pool) *ProfileHandler {
	return &ProfileHandler{db: db}
}

func (h *ProfileHandler) ProfilePage(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)
	username := session.Get("username").(string)

	accounts, err := models.ListLinkedAccounts(c.Request.Context(), h.db, userID)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "profile/index", gin.H{
			"error": "Failed to load linked accounts",
		})
		return
	}

	data, _, _ := models.GetAvatarByUserID(c.Request.Context(), h.db, userID)
	hasAvatar := len(data) > 0

	gravatarURL := fmt.Sprintf(
		"https://www.gravatar.com/avatar/%x?d=identicon&s=128",
		md5.Sum([]byte(strings.ToLower(username))),
	)

	var steamAccount *models.LinkedAccount
	for _, acc := range accounts {
		if acc.Provider == "steam" {
			steamAccount = acc
			break
		}
	}

	c.HTML(http.StatusOK, "profile/index", gin.H{
		"username":        username,
		"activeNav":       "profile",
		"hasAvatar":       hasAvatar,
		"gravatarURL":     gravatarURL,
		"steamAccount":    steamAccount,
		"error":           c.Query("error"),
		"passwordError":   c.Query("password_error"),
		"passwordSuccess": c.Query("password_success") == "1",
	})
}

func (h *ProfileHandler) ChangePassword(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)

	currentPassword := c.PostForm("current_password")
	newPassword := c.PostForm("new_password")
	confirmPassword := c.PostForm("confirm_password")

	redirectWithError := func(message string) {
		c.Redirect(http.StatusSeeOther, "/profile?password_error="+message+"#password")
	}

	if len(newPassword) < 5 {
		redirectWithError("Password+must+be+at+least+5+characters")
		return
	}

	if newPassword != confirmPassword {
		redirectWithError("Passwords+do+not+match")
		return
	}

	user, err := models.GetUserByID(c.Request.Context(), h.db, userID)
	if err != nil || !user.CheckPassword(currentPassword) {
		redirectWithError("Current+password+is+incorrect")
		return
	}

	if err := models.UpdatePassword(c.Request.Context(), h.db, userID, newPassword); err != nil {
		redirectWithError("Failed+to+update+password")
		return
	}

	c.Redirect(http.StatusSeeOther, "/profile?password_success=1#password")
}

func (h *ProfileHandler) UploadAvatar(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)

	file, err := c.FormFile("avatar")
	if err != nil {
		c.Redirect(http.StatusSeeOther, "/profile?error=No+file+selected")
		return
	}

	if file.Size > 2*1024*1024 {
		c.Redirect(http.StatusSeeOther, "/profile?error=File+too+large")
		return
	}

	src, err := file.Open()
	if err != nil {
		c.Redirect(http.StatusSeeOther, "/profile?error=Failed+to+read+file")
		return
	}
	defer func() { _ = src.Close() }()

	data, err := io.ReadAll(src)
	if err != nil {
		c.Redirect(http.StatusSeeOther, "/profile?error=Failed+to+read+file")
		return
	}

	contentType := file.Header.Get("Content-Type")
	validMIMEs := []string{"image/jpeg", "image/png", "image/gif", "image/webp"}
	valid := false
	for _, m := range validMIMEs {
		if contentType == m {
			valid = true
			break
		}
	}
	if !valid {
		c.Redirect(http.StatusSeeOther, "/profile?error=Invalid+image+format")
		return
	}

	err = models.UpdateAvatar(c.Request.Context(), h.db, userID, data, contentType)
	if err != nil {
		c.Redirect(http.StatusSeeOther, "/profile?error=Failed+to+save+avatar")
		return
	}

	c.Redirect(http.StatusSeeOther, "/profile")
}

func (h *ProfileHandler) ServeAvatar(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)

	data, mime, err := models.GetAvatarByUserID(c.Request.Context(), h.db, userID)
	if err != nil {
		if err == pgx.ErrNoRows {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	if len(data) == 0 {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}

	c.Data(http.StatusOK, mime, data)
}
