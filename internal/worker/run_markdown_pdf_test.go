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
