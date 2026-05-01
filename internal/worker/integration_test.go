//go:build integration

package worker_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/julianshen/bi/internal/worker"
)

func skipNoLOK(t *testing.T) {
	t.Helper()
	if os.Getenv("LOK_PATH") == "" {
		t.Skip("LOK_PATH not set")
	}
	if runtime.GOOS == "windows" {
		t.Skip("not supported on windows")
	}
}

func loadFixture(t *testing.T, name string) string {
	t.Helper()
	root := projectRoot(t)
	return filepath.Join(root, "testdata", name)
}

func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("go.mod not found")
	return ""
}

// TestRealConversion exercises the worker pool against a real LibreOffice
// install. Subtests share a single Pool because lok permits only one
// lok_init per process — creating multiple Pools in the same test binary
// hangs Pool.Close on the second teardown. If you want to add another
// real-LO test, add it as a subtest here rather than as a top-level
// TestX function, otherwise CI will time out at the 600s test deadline.
func TestRealConversion(t *testing.T) {
	skipNoLOK(t)
	cfg := worker.Config{
		LOKPath:        os.Getenv("LOK_PATH"),
		Workers:        1,
		QueueDepth:     1,
		ConvertTimeout: 30 * time.Second,
	}
	p, err := worker.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })

	t.Run("PDF", func(t *testing.T) {
		res, err := p.Run(context.Background(), worker.Job{
			InPath: loadFixture(t, "health.docx"),
			Format: worker.FormatPDF,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(res.OutPath) })
		if res.MIME != "application/pdf" {
			t.Errorf("MIME = %q", res.MIME)
		}
		body, err := os.ReadFile(res.OutPath)
		if err != nil {
			t.Fatal(err)
		}
		if len(body) < 4 || string(body[:4]) != "%PDF" {
			end := 20
			if len(body) < end {
				end = len(body)
			}
			t.Errorf("output is not a PDF: %x", body[:end])
		}
	})

	t.Run("MarkdownMarp", func(t *testing.T) {
		res, err := p.Run(context.Background(), worker.Job{
			InPath:         loadFixture(t, "health.pptx"),
			Format:         worker.FormatMarkdown,
			MarkdownImages: worker.MarkdownImagesEmbed,
			MarkdownMarp:   true,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(res.OutPath) })

		body, err := os.ReadFile(res.OutPath)
		if err != nil {
			t.Fatal(err)
		}
		s := string(body)
		if !strings.HasPrefix(s, "---\nmarp: true\n---\n") {
			t.Errorf("output missing Marp front-matter: %.200q", s)
		}
		// health.pptx has 2 slides → expect ≥2 occurrences of "\n---\n"
		// (front-matter close + ≥1 between slides).
		if c := strings.Count(s, "\n---\n"); c < 2 {
			t.Errorf("expected ≥2 `---` lines (front-matter close + slide breaks); got %d in:\n%s", c, s)
		}
	})

	t.Run("PDFInputPNG", func(t *testing.T) {
		res, err := p.Run(context.Background(), worker.Job{
			InPath: loadFixture(t, "health.pdf"),
			Format: worker.FormatPNG,
			Page:   0,
			DPI:    1.0,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(res.OutPath) })
		if res.MIME != "image/png" {
			t.Errorf("MIME = %q, want image/png", res.MIME)
		}
		body, err := os.ReadFile(res.OutPath)
		if err != nil {
			t.Fatal(err)
		}
		if len(body) < 8 || string(body[:8]) != "\x89PNG\r\n\x1a\n" {
			end := 16
			if len(body) < end {
				end = len(body)
			}
			t.Errorf("output is not a PNG: %x", body[:end])
		}
	})

	t.Run("PDFInputMarkdown", func(t *testing.T) {
		res, err := p.Run(context.Background(), worker.Job{
			InPath:         loadFixture(t, "health.pdf"),
			Format:         worker.FormatMarkdown,
			MarkdownImages: worker.MarkdownImagesEmbed,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(res.OutPath) })
		body, err := os.ReadFile(res.OutPath)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), "Hello PDF") {
			t.Errorf("output missing fixture text 'Hello PDF': %.500q", body)
		}
	})

	t.Run("SimpleDOCX", func(t *testing.T) {
		res, err := p.Run(context.Background(), worker.Job{
			InPath: loadFixture(t, "simple.docx"),
			Format: worker.FormatPDF,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(res.OutPath) })
		if res.MIME != "application/pdf" {
			t.Errorf("MIME = %q, want application/pdf", res.MIME)
		}
	})

	t.Run("SimpleXLSX", func(t *testing.T) {
		res, err := p.Run(context.Background(), worker.Job{
			InPath: loadFixture(t, "simple.xlsx"),
			Format: worker.FormatPDF,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(res.OutPath) })
		if res.MIME != "application/pdf" {
			t.Errorf("MIME = %q, want application/pdf", res.MIME)
		}
	})

	t.Run("SimpleODT", func(t *testing.T) {
		res, err := p.Run(context.Background(), worker.Job{
			InPath: loadFixture(t, "simple.odt"),
			Format: worker.FormatPDF,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(res.OutPath) })
		if res.MIME != "application/pdf" {
			t.Errorf("MIME = %q, want application/pdf", res.MIME)
		}
	})

	// NOTE: encrypted.docx and corrupt.docx fixtures exist in testdata/ but
	// are not exercised here because LibreOffice 7.x on Ubuntu 24.04 does
	// not reliably return password-required for msoffcrypto-generated
	// encrypted files, and truncated zip files can hang the LO load path
	// for the full ConvertTimeout. The error classification paths are
	// covered by unit tests (TestClassify, run_*_test.go).
}
