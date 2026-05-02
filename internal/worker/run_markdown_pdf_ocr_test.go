package worker

import (
	"strings"
	"testing"
)

func TestPageNeedsOCR(t *testing.T) {
	cases := []struct {
		name      string
		mode      OCRMode
		text      string
		threshold int
		want      bool
	}{
		{"never short", OCRNever, "", 16, false},
		{"never long", OCRNever, strings.Repeat("x", 100), 16, false},
		{"always short", OCRAlways, "", 16, true},
		{"always long", OCRAlways, strings.Repeat("x", 100), 16, true},
		{"auto empty", OCRAuto, "", 16, true},
		{"auto whitespace", OCRAuto, "   \n\t  ", 16, true},
		{"auto below", OCRAuto, "short", 16, true},
		{"auto at threshold", OCRAuto, strings.Repeat("x", 16), 16, false},
		{"auto above", OCRAuto, strings.Repeat("x", 100), 16, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := pageNeedsOCR(c.mode, c.text, c.threshold)
			if got != c.want {
				t.Errorf("pageNeedsOCR(%v, %q, %d) = %v, want %v", c.mode, c.text, c.threshold, got, c.want)
			}
		})
	}
}
