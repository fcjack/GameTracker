package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jacksoncoelho/game-tracker/internal/i18n"
	"github.com/jacksoncoelho/game-tracker/internal/models"
)

const registrationDisabledMessage = "auth.registration_disabled"

type AuthHandler struct {
	db                  *pgxpool.Pool
	registrationEnabled bool
}

func NewAuthHandler(db *pgxpool.Pool, registrationEnabled bool) *AuthHandler {
	return &AuthHandler{db: db, registrationEnabled: registrationEnabled}
}

func (h *AuthHandler) loginTemplateData(c *gin.Context, extra gin.H) gin.H {
	data := ViewData(c, gin.H{"RegistrationEnabled": h.registrationEnabled})
	for k, v := range extra {
		data[k] = v
	}
	return data
}

func (h *AuthHandler) LoginPage(c *gin.Context) {
	if sessions.Default(c).Get("user_id") != nil {
		c.Redirect(http.StatusFound, "/dashboard")
		return
	}
	c.HTML(http.StatusOK, "auth/login", h.loginTemplateData(c, nil))
}

func (h *AuthHandler) Login(c *gin.Context) {
	username := c.PostForm("username")
	password := c.PostForm("password")

	user, err := models.GetUserByUsername(c.Request.Context(), h.db, username)
	if err != nil || !user.CheckPassword(password) {
		c.HTML(http.StatusUnauthorized, "auth/login", h.loginTemplateData(c, gin.H{
			"error": "error.invalid_credentials",
		}))
		return
	}

	session := sessions.Default(c)
	session.Set("user_id", user.ID)
	session.Set("username", user.Username)
	locale := i18n.Normalize(user.Locale)
	if locale == "" {
		locale = i18n.LocaleEN
	}
	SetSessionLocale(session, locale)
	if err := session.Save(); err != nil {
		c.HTML(http.StatusInternalServerError, "auth/login", h.loginTemplateData(c, gin.H{
			"error": "error.session_failed",
		}))
		return
	}

	c.Redirect(http.StatusFound, "/dashboard")
}

func (h *AuthHandler) RegisterPage(c *gin.Context) {
	if sessions.Default(c).Get("user_id") != nil {
		c.Redirect(http.StatusFound, "/dashboard")
		return
	}
	if !h.registrationEnabled {
		c.Redirect(http.StatusFound, "/login")
		return
	}
	c.HTML(http.StatusOK, "auth/register", ViewData(c, gin.H{"RegistrationEnabled": true}))
}

func (h *AuthHandler) Register(c *gin.Context) {
	if !h.registrationEnabled {
		c.HTML(http.StatusForbidden, "auth/register", ViewData(c, gin.H{
			"RegistrationEnabled": false,
			"error":               registrationDisabledMessage,
		}))
		return
	}

	username := c.PostForm("username")
	password := c.PostForm("password")
	confirm := c.PostForm("confirm_password")

	if len(username) < 3 {
		c.HTML(http.StatusBadRequest, "auth/register", ViewData(c, gin.H{
			"RegistrationEnabled": true,
			"error":               "error.username_too_short",
		}))
		return
	}

	if len(password) < 5 {
		c.HTML(http.StatusBadRequest, "auth/register", ViewData(c, gin.H{
			"RegistrationEnabled": true,
			"error":               "error.password_too_short",
		}))
		return
	}

	if password != confirm {
		c.HTML(http.StatusBadRequest, "auth/register", ViewData(c, gin.H{
			"RegistrationEnabled": true,
			"error":               "error.passwords_mismatch",
		}))
		return
	}

	user, err := models.CreateUser(c.Request.Context(), h.db, username, password)
	if err != nil {
		c.HTML(http.StatusBadRequest, "auth/register", ViewData(c, gin.H{
			"RegistrationEnabled": true,
			"error":               "error.username_taken",
		}))
		return
	}

	session := sessions.Default(c)
	session.Set("user_id", user.ID)
	session.Set("username", user.Username)
	locale := LocaleFromContext(c)
	SetSessionLocale(session, locale)
	_ = models.UpdateUserLocale(c.Request.Context(), h.db, user.ID, locale)
	if err := session.Save(); err != nil {
		c.HTML(http.StatusInternalServerError, "auth/register", ViewData(c, gin.H{
			"RegistrationEnabled": true,
			"error":               "error.session_failed",
		}))
		return
	}

	c.Redirect(http.StatusFound, "/dashboard")
}

func (h *AuthHandler) Logout(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	_ = session.Save()
	c.Redirect(http.StatusFound, "/")
}

func dashboardYear(c *gin.Context) int {
	year := time.Now().Year()
	if yearStr := c.Query("year"); yearStr != "" {
		if y, err := strconv.Atoi(yearStr); err == nil && y >= 1970 && y <= 2100 {
			year = y
		}
	}
	return year
}

func dashboardYearOptions(ctx context.Context, db *pgxpool.Pool, userID int64) []int {
	currentYear := time.Now().Year()
	years, err := models.ListCompletionYears(ctx, db, userID)
	if err != nil || len(years) == 0 {
		return []int{currentYear}
	}
	seen := map[int]bool{currentYear: true}
	options := []int{currentYear}
	for _, y := range years {
		if !seen[y] {
			seen[y] = true
			options = append(options, y)
		}
	}
	return options
}

func dashboardStatsMap(ctx context.Context, db *pgxpool.Pool, userID int64, year int) map[string]int {
	stats, err := models.GetGameStatistics(ctx, db, userID)
	if err != nil {
		stats = map[string]int{
			"owned":     0,
			"playing":   0,
			"completed": 0,
			"dropped":   0,
		}
	}
	completedInYear, err := models.GetCompletedCountByYear(ctx, db, userID, year)
	if err != nil {
		completedInYear = 0
	}
	return map[string]int{
		"Playing":         stats["playing"],
		"Completed":       stats["completed"],
		"CompletedInYear": completedInYear,
		"Backlog":         stats["owned"],
		"Dropped":         stats["dropped"],
	}
}

func (h *AuthHandler) Dashboard(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)
	username := session.Get("username").(string)

	year := dashboardYear(c)
	stats := dashboardStatsMap(c.Request.Context(), h.db, userID, year)
	hasGames := stats["Playing"]+stats["Completed"]+stats["Backlog"]+stats["Dropped"] > 0

	c.HTML(http.StatusOK, "dashboard/index", ViewData(c, gin.H{
		"username":       username,
		"activeNav":      "dashboard",
		"hasGames":       hasGames,
		"stats":          stats,
		"year":           year,
		"years":          dashboardYearOptions(c.Request.Context(), h.db, userID),
		"libraryGridURL": "/library/games?filter=active",
	}))
}

func (h *AuthHandler) DashboardStats(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)

	year := dashboardYear(c)
	c.HTML(http.StatusOK, "dashboard/stats", ViewData(c, gin.H{
		"stats": dashboardStatsMap(c.Request.Context(), h.db, userID, year),
		"year":  year,
		"years": dashboardYearOptions(c.Request.Context(), h.db, userID),
	}))
}
