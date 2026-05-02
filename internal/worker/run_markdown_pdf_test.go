package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExtractPDFText(t *testing.T) {
	// Repo-relative path: tests run from internal/worker/, fixture lives at <repo>/testdata/.
	path := filepath.Join("..", "..", "testdata", "health.pdf")
	got, err := extractPDFText(path)
	if err != nil {
		t.Fatalf("extractPDFText: %v", err)
	}
	if !strings.Contains(string(got), "Hello PDF") {
		t.Errorf("extracted text missing fixture content; got %q", got)
	}
}

// TestExtractPDFTextMultiPage exercises the inter-page separator
// branch (the single-page health.pdf fixture cannot). A regression
// that drops the blank-line separator or reorders pages would smush
// "Page One Body" and "Page Two Body" together.
func TestExtractPDFTextMultiPage(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "multi-page.pdf")
	got, err := extractPDFText(path)
	if err != nil {
		t.Fatalf("extractPDFText: %v", err)
	}
	s := string(got)
	idx1 := strings.Index(s, "Page One Body")
	idx2 := strings.Index(s, "Page Two Body")
	if idx1 < 0 || idx2 < 0 {
		t.Fatalf("expected both page bodies in output; got %q", s)
	}
	if idx1 >= idx2 {
		t.Errorf("expected page 1 before page 2; got %q", s)
	}
	between := s[idx1+len("Page One Body") : idx2]
	if !strings.Contains(between, "\n\n") {
		t.Errorf("missing blank-line separator between pages; got %q", between)
	}
}

func TestExtractPDFTextMissingFile(t *testing.T) {
	_, err := extractPDFText("does-not-exist.pdf")
	if err == nil {
		t.Fatal("want error for missing file")
	}
}

// TestPoolRunMarkdownPDFShortCircuit verifies the PDF input path
// bypasses the office.Load + mdconv pipeline entirely. The fake
// office records loadCalls; if the PDF branch is missing, runMarkdown
// falls through and tries to load the .pdf via fakeOffice (which
// would return success but the fakeDocument.SaveAs would write empty
// HTML and the fakeMD.Convert would be invoked).
func TestPoolRunMarkdownPDFShortCircuit(t *testing.T) {
	office := &fakeOffice{}
	p, _ := newWithOffice(Config{Workers: 1, QueueDepth: 1, ConvertTimeout: time.Second}, office)
	t.Cleanup(func() { _ = p.Close() })
	md := &fakeMD{}
	p.setMarkdown(md)

	pdfPath := filepath.Join("..", "..", "testdata", "health.pdf")
	res, err := p.Run(context.Background(), Job{InPath: pdfPath, Format: FormatMarkdown})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(res.OutPath) })

	if len(office.loadCalls) != 0 {
		t.Errorf("office.Load called with %v; PDF path must short-circuit before LO", office.loadCalls)
	}
	if md.got != nil {
		t.Errorf("mdconv.Convert called with %q; PDF path must skip mdconv", md.got)
	}
	if res.MIME != "text/markdown" {
		t.Errorf("MIME = %q, want text/markdown", res.MIME)
	}
	body, err := os.ReadFile(res.OutPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Hello PDF") {
		t.Errorf("output missing extracted text; got %q", body)
	}
}

func TestIsEncryptedPDFErr(t *testing.T) {
	cases := map[string]bool{
		"":                               false,
		"some random error":              false,
		"file is encrypted":              true,
		"PDF requires a password":        true,
		"Encrypted document, can't open": true,
		"failed to decode XREF":          false,
		"PASSWORD REQUIRED":              true,
	}
	for msg, want := range cases {
		var err error
		if msg != "" {
			err = errString(msg)
		}
		if got := isEncryptedPDFErr(err); got != want {
			t.Errorf("isEncryptedPDFErr(%q) = %v, want %v", msg, got, want)
		}
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestIsPDFInput(t *testing.T) {
	cases := map[string]bool{
		"foo.pdf":               true,
		"foo.PDF":               true,
		"/tmp/bi-in-1234.pdf":   true,
		"foo.docx":              false,
		"foo":                   false,
		"":                      false,
		"foo.pdf.docx":          false,
		"/tmp/has.dots/foo.PDF": true,
	}
	for in, want := range cases {
		if got := isPDFInput(in); got != want {
			t.Errorf("isPDFInput(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestExtractPDFPages(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "multi-page.pdf")
	pages, err := extractPDFPages(path)
	if err != nil {
		t.Fatalf("extractPDFPages: %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("got %d pages, want 2", len(pages))
	}
	if !strings.Contains(pages[0], "Page One Body") {
		t.Errorf("page 0 = %q, want 'Page One Body'", pages[0])
	}
	if !strings.Contains(pages[1], "Page Two Body") {
		t.Errorf("page 1 = %q, want 'Page Two Body'", pages[1])
	}
}

func TestPoolRunMarkdownPDFWithOCREngineNoOpForDigitalPDF(t *testing.T) {
	office := &fakeOffice{}
	eng := &fakeOCR{}
	cfg := Config{
		Workers: 1, QueueDepth: 1, ConvertTimeout: time.Second,
		OCR: eng, OCRTextThreshold: 8, OCRDPI: 300, // low threshold: "Page One Body" is 13 chars, so no OCR needed
	}
	p, _ := newWithOffice(cfg, office)
	t.Cleanup(func() { _ = p.Close() })
	p.setMarkdown(&fakeMD{})

	pdfPath := filepath.Join("..", "..", "testdata", "multi-page.pdf")
	res, err := p.Run(context.Background(), Job{
		InPath: pdfPath, Format: FormatMarkdown, OCRMode: OCRAuto, OCRLang: "eng",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(res.OutPath) })
	if len(eng.calls) != 0 {
		t.Errorf("engine called for digital PDF with sufficient text: %d", len(eng.calls))
	}
	body, _ := os.ReadFile(res.OutPath)
	if !strings.Contains(string(body), "Page One Body") {
		t.Errorf("missing extracted text: %q", body)
	}
}

// TestPoolRunMarkdownPDFAutoMixedPages verifies that OCRAuto routes
// through assembleMarkdownWithOCR so pages with no text layer get
// OCR'd while text-bearing pages don't. This guards against a past
// regression where OCRAuto bypassed OCR entirely.
//
// We can't easily synthesise a "scanned page" in a fixture without
// LibreOffice; instead we use a fake document that returns empty
// pages from extractPDFPages by overriding via an empty 2-page PDF.
// Because we don't have such a fixture, we assert the OCR pipeline
// is invoked (engine called at least once) when OCRAuto is set on
// any non-empty input — i.e. the test fails if the function takes
// the "skip OCR entirely" shortcut on OCRAuto.
//
// Construction: we set OCRTextThreshold high enough that every page
// of multi-page.pdf falls under the threshold; that guarantees every
// page becomes an OCR call.
func TestPoolRunMarkdownPDFAutoForcesOCROnShortPages(t *testing.T) {
	office := &fakeOffice{}
	eng := &fakeOCR{textsByCall: []string{"OCR1", "OCR2"}}
	cfg := Config{
		Workers: 1, QueueDepth: 1, ConvertTimeout: time.Second,
		OCR: eng, OCRTextThreshold: 100000, OCRDPI: 300, // huge threshold = every page goes through OCR
	}
	p, _ := newWithOffice(cfg, office)
	t.Cleanup(func() { _ = p.Close() })
	p.setMarkdown(&fakeMD{})

	// fakeOffice.Load needs to succeed; default fakeDocument has parts=1, that's fine
	// (we only need rendering not to error).

	pdfPath := filepath.Join("..", "..", "testdata", "multi-page.pdf")
	res, err := p.Run(context.Background(), Job{
		InPath: pdfPath, Format: FormatMarkdown, OCRMode: OCRAuto, OCRLang: "eng",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(res.OutPath) })

	if len(eng.calls) == 0 {
		t.Fatalf("OCRAuto with low-text pages did not invoke OCR (regression)")
	}
}
