package worker

import (
	"context"
	"fmt"
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
