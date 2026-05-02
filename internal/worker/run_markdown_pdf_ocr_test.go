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

func TestRunMarkdownPDFWithOCRMixedPages(t *testing.T) {
	pages := []string{"This page has plenty of real text on it.", "  "}
	doc := &fakeDocument{parts: len(pages)}
	doc.renderBytes = []byte("png")
	eng := &fakeOCR{textsByCall: []string{"OCR_PAGE_2"}}

	resPath, err := assembleMarkdownWithOCR(context.Background(), pages, doc, eng, OCRAuto, "eng", 16, 300)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(resPath) })
	body := readFileForTest(t, resPath)
	if !strings.Contains(body, "real text on it") {
		t.Errorf("missing page 1 text in %q", body)
	}
	if !strings.Contains(body, "OCR_PAGE_2") {
		t.Errorf("missing page 2 OCR text in %q", body)
	}
	if !strings.Contains(body, "<!-- ocr: eng page=2 -->") {
		t.Errorf("missing OCR marker in %q", body)
	}
	if !strings.Contains(body, "\n---\n") {
		t.Errorf("missing page-break separator in %q", body)
	}
	if len(eng.calls) != 1 {
		t.Errorf("ocr calls = %d, want 1", len(eng.calls))
	}
}

func TestRunMarkdownPDFWithOCRAlways(t *testing.T) {
	pages := []string{"plenty of real text", "more real text"}
	doc := &fakeDocument{parts: len(pages)}
	doc.renderBytes = []byte("png")
	eng := &fakeOCR{textsByCall: []string{"O1", "O2"}}

	resPath, err := assembleMarkdownWithOCR(context.Background(), pages, doc, eng, OCRAlways, "eng", 16, 300)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(resPath) })
	body := readFileForTest(t, resPath)
	if strings.Contains(body, "real text") {
		t.Errorf("text-layer leaked under OCRAlways: %q", body)
	}
	if !strings.Contains(body, "O1") || !strings.Contains(body, "O2") {
		t.Errorf("missing OCR text: %q", body)
	}
	if len(eng.calls) != 2 {
		t.Errorf("ocr calls = %d, want 2", len(eng.calls))
	}
}

func TestRunMarkdownPDFWithOCRNever(t *testing.T) {
	pages := []string{"plenty of real text", ""}
	doc := &fakeDocument{parts: len(pages)}
	eng := &fakeOCR{}

	resPath, err := assembleMarkdownWithOCR(context.Background(), pages, doc, eng, OCRNever, "eng", 16, 300)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(resPath) })
	if len(eng.calls) != 0 {
		t.Errorf("ocr called under OCRNever: %d", len(eng.calls))
	}
}

func TestRunMarkdownPDFWithOCRPerPageError(t *testing.T) {
	pages := []string{"", ""}
	doc := &fakeDocument{parts: len(pages)}
	doc.renderBytes = []byte("png")
	eng := &fakeOCR{
		textsByCall: []string{"", "ok"},
		errsByCall:  []error{errors.New("boom"), nil},
	}

	resPath, err := assembleMarkdownWithOCR(context.Background(), pages, doc, eng, OCRAuto, "eng", 16, 300)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(resPath) })
	body := readFileForTest(t, resPath)
	if !strings.Contains(body, "<!-- ocr-error:") {
		t.Errorf("missing per-page error marker in %q", body)
	}
	if !strings.Contains(body, "page=1") {
		t.Errorf("error marker should reference page 1 in %q", body)
	}
	if !strings.Contains(body, "ok") {
		t.Errorf("page 2 OCR text missing in %q", body)
	}
}

func TestRunMarkdownPDFWithOCRAllPagesFail(t *testing.T) {
	pages := []string{"", ""}
	doc := &fakeDocument{parts: len(pages)}
	doc.renderBytes = []byte("png")
	eng := &fakeOCR{errsByCall: []error{errors.New("a"), errors.New("b")}}

	_, err := assembleMarkdownWithOCR(context.Background(), pages, doc, eng, OCRAuto, "eng", 16, 300)
	if !errors.Is(err, ErrOCRFailed) {
		t.Fatalf("err = %v, want ErrOCRFailed", err)
	}
}

func readFileForTest(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
