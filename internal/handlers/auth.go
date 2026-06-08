package handlers

import (
	"context"
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jacksoncoelho/game-tracker/internal/models"
)

type AuthHandler struct {
	db *pgxpool.Pool
}

func NewAuthHandler(db *pgxpool.Pool) *AuthHandler {
	return &AuthHandler{db: db}
}

func (h *AuthHandler) LoginPage(c *gin.Context) {
	if sessions.Default(c).Get("user_id") != nil {
		c.Redirect(http.StatusFound, "/dashboard")
		return
	}
	c.HTML(http.StatusOK, "auth/login", gin.H{})
}

func (h *AuthHandler) Login(c *gin.Context) {
	username := c.PostForm("username")
	password := c.PostForm("password")

	user, err := models.GetUserByUsername(c.Request.Context(), h.db, username)
	if err != nil || !user.CheckPassword(password) {
		c.HTML(http.StatusUnauthorized, "auth/login", gin.H{
			"error": "Invalid username or password",
		})
		return
	}

	session := sessions.Default(c)
	session.Set("user_id", user.ID)
	session.Set("username", user.Username)
	if err := session.Save(); err != nil {
		c.HTML(http.StatusInternalServerError, "auth/login", gin.H{
			"error": "Failed to save session",
		})
		return
	}

	c.Redirect(http.StatusFound, "/dashboard")
}

func (h *AuthHandler) RegisterPage(c *gin.Context) {
	if sessions.Default(c).Get("user_id") != nil {
		c.Redirect(http.StatusFound, "/dashboard")
		return
	}
	c.HTML(http.StatusOK, "auth/register", gin.H{})
}

func (h *AuthHandler) Register(c *gin.Context) {
	username := c.PostForm("username")
	password := c.PostForm("password")
	confirm := c.PostForm("confirm_password")

	if len(username) < 3 {
		c.HTML(http.StatusBadRequest, "auth/register", gin.H{
			"error": "Username must be at least 3 characters",
		})
		return
	}

	if len(password) < 5 {
		c.HTML(http.StatusBadRequest, "auth/register", gin.H{
			"error": "Password must be at least 5 characters",
		})
		return
	}

	if password != confirm {
		c.HTML(http.StatusBadRequest, "auth/register", gin.H{
			"error": "Passwords do not match",
		})
		return
	}

	user, err := models.CreateUser(c.Request.Context(), h.db, username, password)
	if err != nil {
		c.HTML(http.StatusBadRequest, "auth/register", gin.H{
			"error": "Username already taken",
		})
		return
	}

	session := sessions.Default(c)
	session.Set("user_id", user.ID)
	session.Set("username", user.Username)
	if err := session.Save(); err != nil {
		c.HTML(http.StatusInternalServerError, "auth/register", gin.H{
			"error": "Failed to save session",
		})
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

func dashboardStatsMap(ctx context.Context, db *pgxpool.Pool, userID int64) map[string]int {
	stats, err := models.GetGameStatistics(ctx, db, userID)
	if err != nil {
		stats = map[string]int{
			"owned":     0,
			"playing":   0,
			"completed": 0,
			"dropped":   0,
		}
	}
	return map[string]int{
		"Playing":   stats["playing"],
		"Completed": stats["completed"],
		"Backlog":   stats["owned"],
		"Dropped":   stats["dropped"],
	}
}

func (h *AuthHandler) Dashboard(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)
	username := session.Get("username").(string)

	stats := dashboardStatsMap(c.Request.Context(), h.db, userID)
	hasGames := stats["Playing"]+stats["Completed"]+stats["Backlog"]+stats["Dropped"] > 0

	c.HTML(http.StatusOK, "dashboard/index", gin.H{
		"username": username,
		"hasGames": hasGames,
		"stats":    stats,
	})
}

func (h *AuthHandler) DashboardStats(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id").(int64)

	c.HTML(http.StatusOK, "dashboard/stats", gin.H{
		"stats": dashboardStatsMap(c.Request.Context(), h.db, userID),
	})
}
