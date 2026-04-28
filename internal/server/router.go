package server

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/julianshen/bi/internal/worker"
)

type Deps struct {
	Conv           worker.Converter
	Logger         *slog.Logger
	APIToken       string
	MaxUploadBytes int64
}

type Server struct{ deps Deps }

func New(deps Deps) *Server {
	if deps.Logger == nil {
		deps.Logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}
	if deps.MaxUploadBytes == 0 {
		deps.MaxUploadBytes = 100 * 1024 * 1024
	}
	return &Server{deps: deps}
}

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(Recover)
	r.Use(RequestID)
	r.Use(AccessLog(s.deps.Logger))

	// Public
	r.Get("/healthz", s.healthz)

	// Auth-gated conversion routes
	r.Group(func(r chi.Router) {
		r.Use(Auth(s.deps.APIToken))
		r.Use(MaxBytes(s.deps.MaxUploadBytes))
		r.Post("/v1/convert/pdf", s.convertPDF)
	})

	return r
}
