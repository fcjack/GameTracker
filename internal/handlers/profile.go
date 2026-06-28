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
	"github.com/jacksoncoelho/game-tracker/internal/i18n"
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
	locale := LocaleFromContext(c)

	base := h.profileTemplateBase(c, userID, username, locale)

	accounts, err := models.ListLinkedAccounts(c.Request.Context(), h.db, userID)
	if err != nil {
		base["error"] = "error.load_accounts"
		c.HTML(http.StatusInternalServerError, "profile/index", ViewData(c, base))
		return
	}

	var steamAccount *models.LinkedAccount
	var xboxAccount *models.LinkedAccount
	for _, acc := range accounts {
		switch acc.Provider {
		case "steam":
			steamAccount = acc
		case "xbox":
			xboxAccount = acc
		}
	}

	var steamImportJob *models.ImportJob
	if steamAccount != nil {
		job, err := models.GetLatestImportJob(c.Request.Context(), h.db, userID, "steam")
		if err == nil {
			steamImportJob = job
		}
	}

	var xboxImportJob *models.ImportJob
	if xboxAccount != nil {
		job, err := models.GetLatestImportJob(c.Request.Context(), h.db, userID, "xbox")
		if err == nil {
			xboxImportJob = job
		}
	}

	base["steamAccount"] = steamAccount
	base["xboxAccount"] = xboxAccount
	base["steamImportJob"] = steamImportJob
	base["xboxImportJob"] = xboxImportJob
	base["importJobSummary"] = i18n.ImportJobSummary(steamImportJob, locale)
	base["xboxImportJobSummary"] = i18n.ImportJobSummary(xboxImportJob, locale)

	c.HTML(http.StatusOK, "profile/index", ViewData(c, base))
}

func (h *ProfileHandler) profileTemplateBase(c *gin.Context, userID int64, username, locale string) gin.H {
	data, _, _ := models.GetAvatarByUserID(c.Request.Context(), h.db, userID)
	hasAvatar := len(data) > 0

	gravatarURL := fmt.Sprintf(
		"https://www.gravatar.com/avatar/%x?d=identicon&s=128",
		md5.Sum([]byte(strings.ToLower(username))),
	)

	steamCleared := 0
	if cleared := c.Query("steam_cleared"); cleared != "" {
		if n, err := fmt.Sscanf(cleared, "%d", &steamCleared); err != nil || n != 1 {
			steamCleared = 0
		}
	}

	xboxCleared := 0
	if cleared := c.Query("xbox_cleared"); cleared != "" {
		if n, err := fmt.Sscanf(cleared, "%d", &xboxCleared); err != nil || n != 1 {
			xboxCleared = 0
		}
	}

	return gin.H{
		"username":        username,
		"activeNav":         "profile",
		"hasAvatar":         hasAvatar,
		"gravatarURL":       gravatarURL,
		"locale":            locale,
		"error":             c.Query("error"),
		"passwordError":     c.Query("password_error"),
		"passwordSuccess":   c.Query("password_success") == "1",
		"localeSuccess":     c.Query("locale_success") == "1",
		"steamCleared":      steamCleared,
		"xboxCleared":       xboxCleared,
	}
}

func (h *ProfileHandler) ChangeLocale(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)

	locale := i18n.Normalize(c.PostForm("locale"))
	if locale == "" {
		c.Redirect(http.StatusSeeOther, "/profile")
		return
	}

	if err := models.UpdateUserLocale(c.Request.Context(), h.db, userID, locale); err != nil {
		c.Redirect(http.StatusSeeOther, "/profile?error=error.update_locale_failed")
		return
	}

	SetSessionLocale(session, locale)
	_ = session.Save()
	c.Redirect(http.StatusSeeOther, "/profile?locale_success=1")
}

func (h *ProfileHandler) ChangePassword(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)

	currentPassword := c.PostForm("current_password")
	newPassword := c.PostForm("new_password")
	confirmPassword := c.PostForm("confirm_password")

	redirectWithError := func(code string) {
		c.Redirect(http.StatusSeeOther, "/profile?password_error="+code+"#password")
	}

	if len(newPassword) < 5 {
		redirectWithError("error.password_too_short")
		return
	}

	if newPassword != confirmPassword {
		redirectWithError("error.passwords_mismatch")
		return
	}

	user, err := models.GetUserByID(c.Request.Context(), h.db, userID)
	if err != nil || !user.CheckPassword(currentPassword) {
		redirectWithError("error.current_password_wrong")
		return
	}

	if err := models.UpdatePassword(c.Request.Context(), h.db, userID, newPassword); err != nil {
		redirectWithError("error.update_password_failed")
		return
	}

	c.Redirect(http.StatusSeeOther, "/profile?password_success=1#password")
}

func (h *ProfileHandler) UploadAvatar(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)

	file, err := c.FormFile("avatar")
	if err != nil {
		c.Redirect(http.StatusSeeOther, "/profile?error=error.no_file")
		return
	}

	if file.Size > 2*1024*1024 {
		c.Redirect(http.StatusSeeOther, "/profile?error=error.file_too_large")
		return
	}

	src, err := file.Open()
	if err != nil {
		c.Redirect(http.StatusSeeOther, "/profile?error=error.read_file")
		return
	}
	defer func() { _ = src.Close() }()

	data, err := io.ReadAll(src)
	if err != nil {
		c.Redirect(http.StatusSeeOther, "/profile?error=error.read_file")
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
		c.Redirect(http.StatusSeeOther, "/profile?error=error.invalid_image")
		return
	}

	err = models.UpdateAvatar(c.Request.Context(), h.db, userID, data, contentType)
	if err != nil {
		c.Redirect(http.StatusSeeOther, "/profile?error=error.save_avatar")
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

	// Avatars change on upload, so revalidate with an ETag instead of a
	// fixed max-age: unchanged avatars short-circuit to a bodyless 304.
	etag := fmt.Sprintf(`"%x"`, md5.Sum(data))
	c.Header("ETag", etag)
	c.Header("Cache-Control", "private, no-cache")
	if c.GetHeader("If-None-Match") == etag {
		c.Status(http.StatusNotModified)
		return
	}

	c.Data(http.StatusOK, mime, data)
}
