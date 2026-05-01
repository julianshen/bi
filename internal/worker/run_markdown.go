package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func (p *Pool) runMarkdown(ctx context.Context, job Job) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if p.md == nil {
		// Reachable from tests that wire the pool via newWithOffice without
		// calling setMarkdown. Production New always sets mdAdapter{}.
		return Result{}, errors.New("worker: markdown converter not wired")
	}
	// PDFs short-circuit: LO's pdfimport flattens pages to embedded
	// images, so the doc.SaveAs("html") → mdconv path returns no text.
	// Use a Go-native PDF reader instead.
	if isPDFInput(job.InPath) {
		text, err := extractPDFText(job.InPath)
		if err != nil {
			return Result{}, fmt.Errorf("%w: %w", ErrMarkdownConversion, err)
		}
		return writePDFMarkdownResult(text)
	}
	doc, err := p.office.Load(job.InPath, job.Password)
	if err != nil {
		return Result{}, Classify(err)
	}
	defer doc.Close()

	htmlFile, err := os.CreateTemp("", "bi-*.html")
	if err != nil {
		return Result{}, fmt.Errorf("worker: create html temp: %w", err)
	}
	htmlPath := htmlFile.Name()
	htmlFile.Close()
	defer os.Remove(htmlPath)

	if err := doc.SaveAs(htmlPath, "html", ""); err != nil {
		return Result{}, Classify(err)
	}
	htmlBytes, err := os.ReadFile(htmlPath)
	if err != nil {
		return Result{}, fmt.Errorf("worker: read html: %w", err)
	}
	mdBytes, err := p.md.Convert(htmlBytes, job.MarkdownImages, filepath.Dir(htmlPath), job.MarkdownMarp)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %w", ErrMarkdownConversion, err)
	}

	out, err := os.CreateTemp("", "bi-*.md")
	if err != nil {
		return Result{}, fmt.Errorf("worker: create md temp: %w", err)
	}
	outPath := out.Name()
	if _, err := out.Write(mdBytes); err != nil {
		out.Close()
		_ = os.Remove(outPath)
		return Result{}, fmt.Errorf("worker: write md: %w", err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(outPath)
		return Result{}, fmt.Errorf("worker: close md: %w", err)
	}
	return Result{OutPath: outPath, MIME: "text/markdown"}, nil
}
