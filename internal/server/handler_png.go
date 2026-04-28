package server

import (
	"net/http"
	"strconv"

	"github.com/julianshen/bi/internal/worker"
)

func (s *Server) convertPNG(w http.ResponseWriter, r *http.Request) {
	page, dpi, err := parsePNGParams(r, 1.0)
	if err != nil {
		WriteProblem(w, r.URL.Path, RequestIDFrom(r.Context()), err)
		return
	}
	s.handleConversion(w, r, func() worker.Job {
		return worker.Job{
			Format:   worker.FormatPNG,
			Page:     page,
			DPI:      dpi,
			Password: r.Header.Get("X-Bi-Password"),
		}
	})
}

func (s *Server) thumbnail(w http.ResponseWriter, r *http.Request) {
	page, dpi, err := parsePNGParams(r, 0.5)
	if err != nil {
		WriteProblem(w, r.URL.Path, RequestIDFrom(r.Context()), err)
		return
	}
	s.handleConversion(w, r, func() worker.Job {
		return worker.Job{
			Format:   worker.FormatPNG,
			Page:     page,
			DPI:      dpi,
			Password: r.Header.Get("X-Bi-Password"),
		}
	})
}

func parsePNGParams(r *http.Request, defaultDPI float64) (page int, dpi float64, err error) {
	page = 0
	dpi = defaultDPI
	if v := r.URL.Query().Get("page"); v != "" {
		n, perr := strconv.Atoi(v)
		if perr != nil {
			return 0, 0, ErrBadQuery{Param: "page", Value: v}
		}
		page = n
	}
	if v := r.URL.Query().Get("dpi"); v != "" {
		f, perr := strconv.ParseFloat(v, 64)
		if perr != nil {
			return 0, 0, ErrBadQuery{Param: "dpi", Value: v}
		}
		dpi = f
	}
	return page, dpi, nil
}
