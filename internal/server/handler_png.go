package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/julianshen/bi/internal/worker"
)

func (s *Server) convertPNG(w http.ResponseWriter, r *http.Request) {
	s.handlePNGLike(w, r, 1.0)
}

func (s *Server) thumbnail(w http.ResponseWriter, r *http.Request) {
	s.handlePNGLike(w, r, 0.5)
}

func (s *Server) handlePNGLike(w http.ResponseWriter, r *http.Request, defaultDPI float64) {
	page, pages, cols, rows, dpi, err := parsePNGParams(r, defaultDPI)
	if err != nil {
		WriteProblem(w, r.URL.Path, RequestIDFrom(r.Context()), err)
		return
	}
	s.handleConversion(w, r, worker.Job{
		Format:   worker.FormatPNG,
		Page:     page,
		Pages:    pages,
		GridCols: cols,
		GridRows: rows,
		DPI:      dpi,
		Password: r.Header.Get("X-Bi-Password"),
	})
}

const (
	minDPI = 0.1
	maxDPI = 4.0
)

func parsePNGParams(r *http.Request, defaultDPI float64) (page int, pages []int, cols int, rows int, dpi float64, err error) {
	dpi = defaultDPI
	q := r.URL.Query()
	if v := q.Get("page"); v != "" {
		if q.Get("pages") != "" {
			return 0, nil, 0, 0, 0, ErrBadQuery{Param: "pages", Value: q.Get("pages")}
		}
		n, perr := strconv.Atoi(v)
		if perr != nil {
			return 0, nil, 0, 0, 0, ErrBadQuery{Param: "page", Value: v}
		}
		page = n
	}
	if v := q.Get("pages"); v != "" {
		parsed, perr := parsePageList(v)
		if perr != nil {
			return 0, nil, 0, 0, 0, ErrBadQuery{Param: "pages", Value: v}
		}
		pages = parsed
	}
	if v := q.Get("layout"); v != "" {
		if len(pages) == 0 {
			return 0, nil, 0, 0, 0, ErrBadQuery{Param: "layout", Value: v}
		}
		c, r, perr := parseGridLayout(v)
		if perr != nil {
			return 0, nil, 0, 0, 0, ErrBadQuery{Param: "layout", Value: v}
		}
		if len(pages) > c*r {
			return 0, nil, 0, 0, 0, ErrBadQuery{Param: "layout", Value: v}
		}
		cols, rows = c, r
	} else if len(pages) > 0 {
		cols, rows = len(pages), 1
	}
	if v := q.Get("dpi"); v != "" {
		f, perr := strconv.ParseFloat(v, 64)
		if perr != nil {
			return 0, nil, 0, 0, 0, ErrBadQuery{Param: "dpi", Value: v}
		}
		if f < minDPI || f > maxDPI {
			return 0, nil, 0, 0, 0, ErrBadQuery{Param: "dpi", Value: v}
		}
		dpi = f
	}
	return page, pages, cols, rows, dpi, nil
}

func parsePageList(v string) ([]int, error) {
	parts := strings.Split(v, ",")
	pages := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, strconv.ErrSyntax
		}
		page, err := strconv.Atoi(part)
		if err != nil {
			return nil, err
		}
		pages = append(pages, page)
	}
	return pages, nil
}

func parseGridLayout(v string) (cols int, rows int, err error) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(v)), "x")
	if len(parts) != 2 {
		return 0, 0, strconv.ErrSyntax
	}
	cols, err = strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, err
	}
	rows, err = strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, err
	}
	if cols <= 0 || rows <= 0 {
		return 0, 0, strconv.ErrSyntax
	}
	return cols, rows, nil
}
