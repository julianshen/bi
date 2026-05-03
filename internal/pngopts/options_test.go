package pngopts

import (
	"strconv"
	"strings"
	"testing"
)

func TestParsePageListRejectsTooManyPages(t *testing.T) {
	raw := strings.Repeat("0,", MaxSelectedPages) + "0"
	if _, err := ParsePageList(raw); err == nil {
		t.Fatal("want error")
	}
}

func TestValidateLayoutRejectsTooManyCells(t *testing.T) {
	err := ValidateLayout(1, Layout{Cols: MaxGridCells + 1, Rows: 1})
	if err != strconv.ErrSyntax {
		t.Fatalf("err = %v, want strconv.ErrSyntax", err)
	}
}
