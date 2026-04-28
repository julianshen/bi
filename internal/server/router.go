package server

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/julianshen/bi/internal/worker"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Deps struct {
	Conv           worker.Converter
	Logger         *slog.Logger
	APIToken       string
	MaxUploadBytes int64
	ReadyzTTL      time.Duration
	Registry       prometheus.Registerer
	Gatherer       prometheus.Gatherer
	Metrics        *Metrics
}

type Server struct {
	deps    Deps
	readyzC readyzCache
}

func New(deps Deps) *Server {
	if deps.Logger == nil {
		deps.Logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}
	if deps.MaxUploadBytes == 0 {
		deps.MaxUploadBytes = 100 * 1024 * 1024
	}
	if deps.ReadyzTTL == 0 {
		deps.ReadyzTTL = 5 * time.Second
	}
	if deps.Registry == nil {
		reg := prometheus.NewRegistry()
		deps.Registry = reg
		deps.Gatherer = reg
	}
	if deps.Gatherer == nil {
		// If a Registry was supplied without a Gatherer, derive one if possible.
		if g, ok := deps.Registry.(prometheus.Gatherer); ok {
			deps.Gatherer = g
		}
	}
	if deps.Metrics == nil {
		deps.Metrics = NewMetrics(deps.Registry)
	}
	return &Server{deps: deps, readyzC: readyzCache{ttl: deps.ReadyzTTL}}
}

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(otelMiddleware)
	r.Use(Recover)
	r.Use(RequestID)
	r.Use(AccessLog(s.deps.Logger))
	r.Use(RequestMetrics(s.deps.Metrics))

	// Public
	r.Get("/healthz", s.healthz)
	r.Get("/readyz", s.readyz)
	r.Handle("/metrics", promhttp.HandlerFor(s.deps.Gatherer, promhttp.HandlerOpts{}))

	// Auth-gated conversion routes
	r.Group(func(r chi.Router) {
		r.Use(Auth(s.deps.APIToken))
		r.Use(MaxBytes(s.deps.MaxUploadBytes))
		r.Post("/v1/convert/pdf", s.convertPDF)
		r.Post("/v1/convert/png", s.convertPNG)
		r.Post("/v1/convert/markdown", s.convertMarkdown)
		r.Post("/v1/thumbnail", s.thumbnail)
	})

	return r
}
