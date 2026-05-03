package server

import (
	"net/http"
	"strconv"

	"github.com/julianshen/bi/internal/pngopts"
	"github.com/julianshen/bi/internal/worker"
)

func (s *Server) convertPNG(w http.ResponseWriter, r *http.Request) {
	s.handlePNGLike(w, r, 1.0)
}

func (s *Server) thumbnail(w http.ResponseWriter, r *http.Request) {
	s.handlePNGLike(w, r, 0.5)
}

func (s *Server) handlePNGLike(w http.ResponseWriter, r *http.Request, defaultDPI float64) {
	params, err := parsePNGParams(r, defaultDPI)
	if err != nil {
		WriteProblem(w, r.URL.Path, RequestIDFrom(r.Context()), err)
		return
	}
	s.handleConversion(w, r, worker.Job{
		Format:   worker.FormatPNG,
		Page:     params.Page,
		Pages:    params.Pages,
		GridCols: params.GridCols,
		GridRows: params.GridRows,
		DPI:      params.DPI,
		Password: r.Header.Get("X-Bi-Password"),
	})
}

const (
	minDPI = 0.1
	maxDPI = 4.0
)

type pngParams struct {
	Page     int
	Pages    []int
	GridCols int
	GridRows int
	DPI      float64
}

func parsePNGParams(r *http.Request, defaultDPI float64) (pngParams, error) {
	params := pngParams{DPI: defaultDPI}
	q := r.URL.Query()
	if v := q.Get("page"); v != "" {
		if q.Get("pages") != "" {
			return pngParams{}, ErrBadQuery{Param: "pages", Value: q.Get("pages")}
		}
		n, perr := strconv.Atoi(v)
		if perr != nil {
			return pngParams{}, ErrBadQuery{Param: "page", Value: v}
		}
		params.Page = n
	}
	if v := q.Get("pages"); v != "" {
		parsed, perr := pngopts.ParsePageList(v)
		if perr != nil {
			return pngParams{}, ErrBadQuery{Param: "pages", Value: v}
		}
		params.Pages = parsed
	}
	if v := q.Get("layout"); v != "" {
		if len(params.Pages) == 0 {
			return pngParams{}, ErrBadQuery{Param: "layout", Value: v}
		}
		layout, perr := pngopts.ParseGridLayout(v)
		if perr != nil {
			return pngParams{}, ErrBadQuery{Param: "layout", Value: v}
		}
		if err := pngopts.ValidateLayout(len(params.Pages), layout); err != nil {
			return pngParams{}, ErrBadQuery{Param: "layout", Value: v}
		}
		params.GridCols = layout.Cols
		params.GridRows = layout.Rows
	} else if len(params.Pages) > 0 {
		layout := pngopts.DefaultLayout(len(params.Pages))
		params.GridCols = layout.Cols
		params.GridRows = layout.Rows
	}
	if v := q.Get("dpi"); v != "" {
		f, perr := strconv.ParseFloat(v, 64)
		if perr != nil {
			return pngParams{}, ErrBadQuery{Param: "dpi", Value: v}
		}
		if f < minDPI || f > maxDPI {
			return pngParams{}, ErrBadQuery{Param: "dpi", Value: v}
		}
		params.DPI = f
	}
	return params, nil
}
