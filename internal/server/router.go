package server

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/julianshen/bi/internal/worker"
)

type Deps struct {
	Conv           worker.Converter
	Logger         *slog.Logger
	APIToken       string
	MaxUploadBytes int64
	ReadyzTTL      time.Duration
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
	return &Server{deps: deps, readyzC: readyzCache{ttl: deps.ReadyzTTL}}
}

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(Recover)
	r.Use(RequestID)
	r.Use(AccessLog(s.deps.Logger))

	// Public
	r.Get("/healthz", s.healthz)
	r.Get("/readyz", s.readyz)

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
