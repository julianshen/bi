package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	Requests   *prometheus.CounterVec
	Convert    *prometheus.HistogramVec
	QueueWait  *prometheus.HistogramVec
	QueueDepth prometheus.Gauge
	WorkerBusy prometheus.Gauge
	LokErrors  *prometheus.CounterVec
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		Requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "bi_requests_total",
			Help: "HTTP requests handled by route and status.",
		}, []string{"path", "status"}),
		Convert: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "bi_conversion_duration_seconds",
			Help:    "End-to-end conversion request latency.",
			Buckets: prometheus.DefBuckets,
		}, []string{"path"}),
		QueueWait: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "bi_queue_wait_seconds",
			Help:    "Time spent waiting in the worker queue.",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 12),
		}, []string{"path"}),
		QueueDepth: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "bi_queue_depth",
			Help: "Approximate worker queue depth.",
		}),
		WorkerBusy: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "bi_worker_busy",
			Help: "Number of workers currently inside lok.",
		}),
		LokErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "bi_lok_errors_total",
			Help: "LOK errors counted by classification kind.",
		}, []string{"kind"}),
	}
	reg.MustRegister(m.Requests, m.Convert, m.QueueWait, m.QueueDepth, m.WorkerBusy, m.LokErrors)
	return m
}

// RequestMetrics returns a chi middleware that records bi_requests_total and
// bi_conversion_duration_seconds for every request.
func RequestMetrics(m *Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: 200}
			next.ServeHTTP(rec, r)
			dur := time.Since(start).Seconds()
			path := r.URL.Path
			m.Requests.WithLabelValues(path, strconv.Itoa(rec.status)).Inc()
			m.Convert.WithLabelValues(path).Observe(dur)
		})
	}
}
