package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/julianshen/bi/internal/worker"
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	Requests        *prometheus.CounterVec
	Convert         *prometheus.HistogramVec
	QueueWaitHist   *prometheus.HistogramVec
	QueueDepthGauge prometheus.Gauge
	WorkerBusyGauge prometheus.Gauge
	LokErrorCounter *prometheus.CounterVec
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		Requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "bi_requests_total",
			Help: "HTTP requests handled by format and status.",
		}, []string{"format", "status"}),
		Convert: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "bi_conversion_duration_seconds",
			Help:    "End-to-end conversion duration (queue wait excluded).",
			Buckets: prometheus.DefBuckets,
		}, []string{"format"}),
		QueueWaitHist: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "bi_queue_wait_seconds",
			Help:    "Time spent waiting in the worker queue.",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 12),
		}, []string{"format"}),
		QueueDepthGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "bi_queue_depth",
			Help: "Approximate worker queue depth.",
		}),
		WorkerBusyGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "bi_worker_busy",
			Help: "Number of workers currently inside lok.",
		}),
		LokErrorCounter: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "bi_lok_errors_total",
			Help: "LOK errors counted by classification kind.",
		}, []string{"kind"}),
	}
	reg.MustRegister(m.Requests, m.Convert, m.QueueWaitHist, m.QueueDepthGauge, m.WorkerBusyGauge, m.LokErrorCounter)
	return m
}

// Instrumenter implementation for worker.Instrumenter.

func (m *Metrics) QueueWait(format worker.Format, d time.Duration) {
	m.QueueWaitHist.WithLabelValues(format.String()).Observe(d.Seconds())
}

func (m *Metrics) ConversionDuration(format worker.Format, d time.Duration) {
	m.Convert.WithLabelValues(format.String()).Observe(d.Seconds())
}

func (m *Metrics) QueueDepth(d int) {
	m.QueueDepthGauge.Set(float64(d))
}

func (m *Metrics) WorkerBusy(delta int) {
	m.WorkerBusyGauge.Add(float64(delta))
}

func (m *Metrics) LokError(kind string) {
	m.LokErrorCounter.WithLabelValues(kind).Inc()
}

// pathToFormat maps a URL path to the format label used in metrics.
func pathToFormat(path string) string {
	switch path {
	case "/v1/convert/pdf":
		return "pdf"
	case "/v1/convert/png":
		return "png"
	case "/v1/convert/markdown":
		return "markdown"
	case "/v1/thumbnail":
		return "thumbnail"
	default:
		return "-"
	}
}

// RequestMetrics returns a chi middleware that records bi_requests_total
// for every request.
func RequestMetrics(m *Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &statusRecorder{ResponseWriter: w, status: 200}
			next.ServeHTTP(rec, r)
			format := pathToFormat(r.URL.Path)
			m.Requests.WithLabelValues(format, strconv.Itoa(rec.status)).Inc()
		})
	}
}
