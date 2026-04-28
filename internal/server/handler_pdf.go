package server

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/julianshen/bi/internal/worker"
)

func (s *Server) convertPDF(w http.ResponseWriter, r *http.Request) {
	s.handleConversion(w, r, func() worker.Job {
		return worker.Job{Format: worker.FormatPDF, Password: r.Header.Get("X-Bi-Password")}
	})
}

// handleConversion is the shared body-capture + worker-dispatch + response-stream
// pipeline used by every conversion handler. Job-shape decisions belong to the
// caller, communicated via build().
func (s *Server) handleConversion(w http.ResponseWriter, r *http.Request, build func() worker.Job) {
	if r.Header.Get("Content-Type") == "" {
		WriteProblem(w, r.URL.Path, RequestIDFrom(r.Context()), ErrMissingContentType)
		return
	}
	tmp, err := os.CreateTemp("", "bi-in-*")
	if err != nil {
		WriteProblem(w, r.URL.Path, RequestIDFrom(r.Context()), err)
		return
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, r.Body); err != nil {
		tmp.Close()
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			WriteProblem(w, r.URL.Path, RequestIDFrom(r.Context()), ErrPayloadTooLarge)
			return
		}
		WriteProblem(w, r.URL.Path, RequestIDFrom(r.Context()), err)
		return
	}
	if err := tmp.Close(); err != nil {
		WriteProblem(w, r.URL.Path, RequestIDFrom(r.Context()), err)
		return
	}

	job := build()
	job.InPath = tmp.Name()

	res, err := s.deps.Conv.Run(r.Context(), job)
	if err != nil {
		WriteProblem(w, r.URL.Path, RequestIDFrom(r.Context()), err)
		return
	}
	defer os.Remove(res.OutPath)

	w.Header().Set("Content-Type", res.MIME)
	if res.TotalPages > 0 {
		w.Header().Set("X-Total-Pages", strconv.Itoa(res.TotalPages))
	}
	f, err := os.Open(res.OutPath)
	if err != nil {
		WriteProblem(w, r.URL.Path, RequestIDFrom(r.Context()), err)
		return
	}
	defer f.Close()
	if info, err := f.Stat(); err == nil {
		w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f)
}
