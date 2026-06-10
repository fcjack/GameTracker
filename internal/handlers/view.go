package handlers

import (
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jacksoncoelho/game-tracker/internal/i18n"
	"github.com/jacksoncoelho/game-tracker/internal/models"
)

const localeSessionKey = "locale"

func LocaleFromContext(c *gin.Context) string {
	if v, ok := c.Get("locale"); ok {
		if locale, ok := v.(string); ok && locale != "" {
			return locale
		}
	}
	return i18n.LocaleEN
}

func ViewData(c *gin.Context, extra gin.H) gin.H {
	locale := LocaleFromContext(c)
	data := gin.H{
		"lang": locale,
		"T":    i18n.NewTranslator(locale),
	}
	for k, v := range extra {
		data[k] = v
	}
	return data
}

func LocaleMiddleware(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		locale := resolveLocale(c, db, session)
		session.Set(localeSessionKey, locale)
		_ = session.Save()
		c.Set("locale", locale)
		c.Next()
	}
}

func resolveLocale(c *gin.Context, db *pgxpool.Pool, session sessions.Session) string {
	if v := session.Get(localeSessionKey); v != nil {
		if loc := i18n.Normalize(v.(string)); loc != "" {
			return loc
		}
	}

	if userID := session.Get("user_id"); userID != nil {
		loc, err := models.GetUserLocale(c.Request.Context(), db, userID.(int64))
		if err == nil {
			if normalized := i18n.Normalize(loc); normalized != "" {
				return normalized
			}
		}
	}

	if loc := i18n.FromAcceptLanguage(c.GetHeader("Accept-Language")); loc != "" {
		return loc
	}

	return i18n.LocaleEN
}

func SetSessionLocale(session sessions.Session, locale string) {
	session.Set(localeSessionKey, locale)
}
