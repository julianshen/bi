package worker

import (
	"strings"
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
