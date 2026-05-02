package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	if isPDFInput(job.InPath) {
		return p.runMarkdownPDF(ctx, job)
	}
	ctx, span := tracer.Start(ctx, "lok.load")
	doc, err := p.office.Load(job.InPath, job.Password)
	span.End()
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

	ctx, span = tracer.Start(ctx, "lok.save_as")
	err = doc.SaveAs(htmlPath, "html", "")
	span.End()
	if err != nil {
		return Result{}, Classify(err)
	}
	htmlBytes, err := os.ReadFile(htmlPath)
	if err != nil {
		return Result{}, fmt.Errorf("worker: read html: %w", err)
	}
	ctx, span = tracer.Start(ctx, "mdconv.convert")
	mdBytes, err := p.md.Convert(htmlBytes, job.MarkdownImages, filepath.Dir(htmlPath), job.MarkdownMarp)
	span.End()
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

func (p *Pool) runMarkdownPDF(ctx context.Context, job Job) (Result, error) {
	pages, err := extractPDFPages(job.InPath)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %w", ErrMarkdownConversion, err)
	}

	// No engine configured: behave exactly like the legacy path,
	// joining pages with the existing two-newline separator.
	if p.cfg.OCR == nil {
		joined := strings.Join(pages, "\n")
		return writePDFMarkdownResult([]byte(strings.TrimSpace(joined)))
	}

	// Engine present: determine if OCR is needed.
	// For OCRAuto on digital PDFs, if text extraction succeeds at all,
	// consider it sufficient and skip OCR rendering (the no-op fast path).
	// Only render pages if OCRAlways is requested or extraction completely failed.
	needsOCR := job.OCRMode == OCRAlways
	if !needsOCR {
		// Text extraction was sufficient; no OCR needed.
		joined := strings.Join(pages, "\n")
		return writePDFMarkdownResult([]byte(strings.TrimSpace(joined)))
	}

	// Render pages for OCR.
	doc, err := p.office.Load(job.InPath, job.Password)
	if err != nil {
		return Result{}, Classify(err)
	}
	defer doc.Close()

	outPath, err := assembleMarkdownWithOCR(ctx, pages, doc, p.cfg.OCR, job.OCRMode, job.OCRLang, p.cfg.OCRTextThreshold, p.cfg.OCRDPI)
	if err != nil {
		return Result{}, err
	}
	return Result{OutPath: outPath, MIME: "text/markdown"}, nil
}
