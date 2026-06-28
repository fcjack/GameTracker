package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jacksoncoelho/game-tracker/internal/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRequestMetricsIncrementsCounter(t *testing.T) {
	t.Parallel()

	r := gin.New()
	r.Use(RequestMetrics())
	r.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	before := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", "/ping", "200"))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	after := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", "/ping", "200"))
	if after != before+1 {
		t.Fatalf("counter = %v, want %v", after, before+1)
	}
}
