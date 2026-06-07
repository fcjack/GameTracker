package handlers

import (
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/jacksoncoelho/game-tracker/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AuthHandler struct {
	db *pgxpool.Pool
}

func NewAuthHandler(db *pgxpool.Pool) *AuthHandler {
	return &AuthHandler{db: db}
}

func (h *AuthHandler) HomePage(c *gin.Context) {
	session := sessions.Default(c)
	if session.Get("user_id") != nil {
		c.Redirect(http.StatusFound, "/dashboard")
		return
	}
	c.HTML(http.StatusOK, "home", gin.H{})
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
	session.Save()

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

	if len(password) < 8 {
		c.HTML(http.StatusBadRequest, "auth/register", gin.H{
			"error": "Password must be at least 8 characters",
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
	session.Save()

	c.Redirect(http.StatusFound, "/dashboard")
}

func (h *AuthHandler) Logout(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	session.Save()
	c.Redirect(http.StatusFound, "/login")
}

func (h *AuthHandler) Dashboard(c *gin.Context) {
	session := sessions.Default(c)
	c.HTML(http.StatusOK, "dashboard/index", gin.H{
		"username": session.Get("username"),
	})
}
