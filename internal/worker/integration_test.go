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

func TestRealConversionPDF(t *testing.T) {
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
}

func TestRealConversionMarkdownMarp(t *testing.T) {
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
}
