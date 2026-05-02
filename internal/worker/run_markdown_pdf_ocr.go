package worker

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/julianshen/bi/internal/ocr"
)

// pageNeedsOCR returns true when the page should be sent through
// OCR rather than emitted from the text-extraction layer.
func pageNeedsOCR(mode OCRMode, text string, threshold int) bool {
	switch mode {
	case OCRNever:
		return false
	case OCRAlways:
		return true
	default: // OCRAuto
		return len(strings.TrimSpace(text)) < threshold
	}
}

// ocrPage renders the given 0-based page via lok and runs OCR on the
// resulting PNG. Returns recognised text, the language string passed
// to the engine (empty for "auto"), and any error.
func ocrPage(ctx context.Context, doc lokDocument, eng ocr.Engine, page0 int, dpi float64, lang string) (string, string, error) {
	if err := doc.InitializeForRendering(""); err != nil {
		return "", "", fmt.Errorf("worker: init render: %w", err)
	}
	png, err := doc.RenderPagePNG(page0, dpi)
	if err != nil {
		return "", "", fmt.Errorf("worker: render page %d: %w", page0+1, err)
	}
	text, err := eng.Recognize(ctx, png, lang)
	if err != nil {
		return "", "", fmt.Errorf("worker: ocr page %d: %w", page0+1, err)
	}
	return text, lang, nil
}

// assembleMarkdownWithOCR walks the per-page text slice and produces
// a markdown document, OCR'ing pages that need it via eng.
//
// Output format:
//
//	<page 1 body>
//
//	---
//
//	<!-- ocr: <lang> page=2 -->
//	<page 2 body>
//
// Per-page OCR failures become "<!-- ocr-error: ... page=N -->" markers;
// the request only fails (with ErrOCRFailed) if every OCR'd page errored.
func assembleMarkdownWithOCR(ctx context.Context, pages []string, doc lokDocument, eng ocr.Engine, mode OCRMode, lang string, threshold int, dpi float64) (string, error) {
	var sb strings.Builder
	ocrAttempts := 0
	ocrFailures := 0

	for i, raw := range pages {
		if i > 0 {
			sb.WriteString("\n---\n\n")
		}
		if !pageNeedsOCR(mode, raw, threshold) {
			sb.WriteString(strings.TrimRight(raw, "\n"))
			sb.WriteByte('\n')
			continue
		}
		ocrAttempts++
		text, usedLang, err := ocrPage(ctx, doc, eng, i, dpi, lang)
		if err != nil {
			ocrFailures++
			fmt.Fprintf(&sb, "<!-- ocr-error: %s page=%d -->\n", sanitiseComment(err.Error()), i+1)
			continue
		}
		markerLang := usedLang
		if markerLang == "" {
			markerLang = "auto"
		}
		fmt.Fprintf(&sb, "<!-- ocr: %s page=%d -->\n", markerLang, i+1)
		sb.WriteString(strings.TrimRight(text, "\n"))
		sb.WriteByte('\n')
	}

	if ocrAttempts > 0 && ocrFailures == ocrAttempts {
		return "", ErrOCRFailed
	}

	out, err := os.CreateTemp("", "bi-*.md")
	if err != nil {
		return "", fmt.Errorf("worker: create md temp: %w", err)
	}
	outPath := out.Name()
	if _, err := out.WriteString(sb.String()); err != nil {
		out.Close()
		_ = os.Remove(outPath)
		return "", fmt.Errorf("worker: write md: %w", err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(outPath)
		return "", fmt.Errorf("worker: close md: %w", err)
	}
	return outPath, nil
}

// sanitiseComment strips characters that would break the HTML comment
// out, so an OCR error message never lets the comment end early.
func sanitiseComment(s string) string {
	s = strings.ReplaceAll(s, "-->", "--&gt;")
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}
