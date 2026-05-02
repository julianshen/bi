package worker

import (
	"context"
	"errors"
	"os"
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

func TestOCRPageExplicitLang(t *testing.T) {
	doc := &fakeDocument{parts: 1}
	doc.renderBytes = []byte("\x89PNG-fake")
	eng := &fakeOCR{textsByCall: []string{"hello"}}

	got, lang, err := ocrPage(context.Background(), doc, eng, 0, 300, "eng")
	if err != nil {
		t.Fatalf("ocrPage: %v", err)
	}
	if got != "hello" {
		t.Errorf("text = %q, want %q", got, "hello")
	}
	if lang != "eng" {
		t.Errorf("lang = %q, want eng", lang)
	}
	if len(eng.calls) != 1 || eng.calls[0].Lang != "eng" {
		t.Errorf("calls = %+v, want one call with lang=eng", eng.calls)
	}
}

func TestOCRPageAutoFallsThroughEmptyLang(t *testing.T) {
	doc := &fakeDocument{parts: 1}
	doc.renderBytes = []byte("png")
	eng := &fakeOCR{textsByCall: []string{"detected"}}

	_, _, err := ocrPage(context.Background(), doc, eng, 0, 300, "")
	if err != nil {
		t.Fatalf("ocrPage: %v", err)
	}
	if len(eng.calls) != 1 || eng.calls[0].Lang != "" {
		t.Errorf("calls[0].Lang = %q, want empty (engine handles auto)", eng.calls[0].Lang)
	}
}

func TestOCRPageRenderError(t *testing.T) {
	doc := &fakeDocument{renderErr: errors.New("render boom")}
	eng := &fakeOCR{}
	_, _, err := ocrPage(context.Background(), doc, eng, 0, 300, "eng")
	if err == nil || !strings.Contains(err.Error(), "render boom") {
		t.Errorf("err = %v, want one wrapping 'render boom'", err)
	}
}

// suppress unused import until 9c tests are added
var _ = os.Remove
