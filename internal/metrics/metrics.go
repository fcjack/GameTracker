package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total HTTP requests processed",
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	ImportJobsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "import_jobs_total",
			Help: "Import jobs by provider and outcome (started, completed, failed)",
		},
		[]string{"provider", "status"},
	)

	ImportJobDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "import_job_duration_seconds",
			Help:    "Wall-clock time to finish an import job",
			Buckets: []float64{1, 5, 10, 30, 60, 120, 300, 600, 1800, 3600},
		},
		[]string{"provider"},
	)

	ImportGamesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "import_games_total",
			Help: "Games processed during imports by result (imported, skipped)",
		},
		[]string{"provider", "result"},
	)

	ImportJobsActive = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "import_jobs_active",
			Help: "Number of import jobs currently running",
		},
		[]string{"provider"},
	)
)

func init() {
	prometheus.MustRegister(
		HTTPRequestsTotal,
		HTTPRequestDuration,
		ImportJobsTotal,
		ImportJobDuration,
		ImportGamesTotal,
		ImportJobsActive,
	)
}

func Handler() http.Handler {
	return promhttp.Handler()
}
