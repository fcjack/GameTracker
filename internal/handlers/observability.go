package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jacksoncoelho/game-tracker/internal/logging"
	"github.com/jacksoncoelho/game-tracker/internal/metrics"
)

func Recovery() gin.HandlerFunc {
	logger := logging.Default()
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				path := routePath(c)
				logger.Error("panic recovered",
					"error", err,
					"method", c.Request.Method,
					"path", path,
				)
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()
		c.Next()
	}
}

func RequestMetrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		path := routePath(c)
		method := c.Request.Method
		status := strconv.Itoa(c.Writer.Status())
		duration := time.Since(start).Seconds()

		metrics.HTTPRequestsTotal.WithLabelValues(method, path, status).Inc()
		metrics.HTTPRequestDuration.WithLabelValues(method, path).Observe(duration)
	}
}

func RequestLogger() gin.HandlerFunc {
	logger := logging.Default()
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		path := routePath(c)
		status := c.Writer.Status()
		latency := time.Since(start)

		attrs := []any{
			"method", c.Request.Method,
			"path", path,
			"status", status,
			"latency_ms", latency.Milliseconds(),
			"client_ip", c.ClientIP(),
		}
		if reqID := c.Writer.Header().Get("X-Request-Id"); reqID != "" {
			attrs = append(attrs, "request_id", reqID)
		}

		switch {
		case path == "/healthz" || path == "/metrics":
			logger.Debug("request", attrs...)
		case status >= 500:
			logger.Error("request", attrs...)
		case status >= 400:
			logger.Warn("request", attrs...)
		default:
			logger.Info("request", attrs...)
		}
	}
}

func routePath(c *gin.Context) string {
	if path := c.FullPath(); path != "" {
		return path
	}
	return c.Request.URL.Path
}
