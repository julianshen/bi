package pngopts

import (
	"strconv"
	"strings"
)

const (
	MaxSelectedPages = 100
	MaxGridCells     = 100
)

type Layout struct {
	Cols int
	Rows int
}

func ParsePageList(v string) ([]int, error) {
	parts := strings.Split(v, ",")
	if len(parts) > MaxSelectedPages {
		return nil, strconv.ErrSyntax
	}
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

func ParseGridLayout(v string) (Layout, error) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(v)), "x")
	if len(parts) != 2 {
		return Layout{}, strconv.ErrSyntax
	}
	cols, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return Layout{}, err
	}
	rows, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return Layout{}, err
	}
	layout := Layout{Cols: cols, Rows: rows}
	if err := ValidateLayout(1, layout); err != nil {
		return Layout{}, err
	}
	return layout, nil
}

func DefaultLayout(pageCount int) Layout {
	return Layout{Cols: pageCount, Rows: 1}
}

func ValidateLayout(pageCount int, layout Layout) error {
	if pageCount <= 0 || layout.Cols <= 0 || layout.Rows <= 0 {
		return strconv.ErrSyntax
	}
	cells := int64(layout.Cols) * int64(layout.Rows)
	if cells > MaxGridCells || int64(pageCount) > cells {
		return strconv.ErrSyntax
	}
	return nil
}
