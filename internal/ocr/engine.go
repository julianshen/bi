package ocr

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SupportedLangs lists the Tesseract language codes this service
// configures. Order is stable: it determines the expansion of "all".
var SupportedLangs = []string{"eng", "jpn", "chi_sim", "chi_tra"}

// LangAuto is the special token meaning "detect script per page".
// Resolved to the empty string at the Engine boundary; the engine
// then uses Tesseract's OSD path.
const LangAuto = "auto"

// LangAll is the special token expanding to all SupportedLangs joined
// with "+", i.e. all languages in one Tesseract pass.
const LangAll = "all"

// ErrUnavailable means the engine is not configured (e.g. tessdata
// missing, or built with -tags=noocr).
var ErrUnavailable = errors.New("ocr: engine unavailable")

// ErrUnsupportedLang is returned by ValidateLangs for any string
// outside the allowlist.
var ErrUnsupportedLang = errors.New("ocr: unsupported language")

// Engine recognises text in PNG-encoded page images.
type Engine interface {
	// Recognize returns plain UTF-8 text for the given image.
	// langs follows Tesseract syntax (e.g. "eng+jpn") or "" for
	// OSD-driven script detection.
	Recognize(ctx context.Context, image []byte, langs string) (string, error)
	// Close releases engine resources.
	Close() error
}

// Config controls Engine construction.
type Config struct {
	// TessdataPath points to the directory containing *.traineddata.
	TessdataPath string
	// Languages is the set of language packs that must be present
	// at startup. Missing files cause New to fail.
	Languages []string
	// DPI is the render DPI used by callers when rasterising pages.
	DPI float64
}

// ValidateLangs checks an HTTP-supplied language string against the
// allowlist. Accepted values:
//
//   - any single SupportedLangs code
//   - "auto" (script detection)
//   - "all" (= eng+jpn+chi_sim+chi_tra)
//   - "+"-joined SupportedLangs codes, e.g. "eng+jpn"
//
// Empty string, unknown codes, mixed case, and stray separators are
// rejected.
func ValidateLangs(s string) error {
	if s == "" {
		return fmt.Errorf("%w: empty", ErrUnsupportedLang)
	}
	if s == LangAuto || s == LangAll {
		return nil
	}
	parts := strings.Split(s, "+")
	if len(parts) == 0 {
		return fmt.Errorf("%w: %q", ErrUnsupportedLang, s)
	}
	for _, p := range parts {
		if p == "" {
			return fmt.Errorf("%w: empty component in %q", ErrUnsupportedLang, s)
		}
		if !isSupported(p) {
			return fmt.Errorf("%w: %q", ErrUnsupportedLang, s)
		}
	}
	return nil
}

// ResolveLangs expands the supplied string to the Tesseract language
// argument the Engine wants. "auto" → "" (OSD), "all" → "+"-joined
// SupportedLangs, anything else is returned unchanged.
func ResolveLangs(s string) (string, error) {
	if err := ValidateLangs(s); err != nil {
		return "", err
	}
	switch s {
	case LangAuto:
		return "", nil
	case LangAll:
		return strings.Join(SupportedLangs, "+"), nil
	default:
		return s, nil
	}
}

func isSupported(code string) bool {
	for _, c := range SupportedLangs {
		if c == code {
			return true
		}
	}
	return false
}

// Probe verifies that tessdataDir contains the language packs
// required at runtime. langs is the list of language codes (without
// the .traineddata suffix); osd.traineddata is always required for
// the "auto" detection path. Probe touches the filesystem only —
// it does not initialise Tesseract, so it is safe to call from the
// parent serve process which has no cgo dependency on Tesseract.
func Probe(tessdataDir string, langs []string) error {
	if tessdataDir == "" {
		return fmt.Errorf("%w: empty tessdata path", ErrUnavailable)
	}
	required := append([]string{"osd"}, langs...)
	for _, l := range required {
		path := filepath.Join(tessdataDir, l+".traineddata")
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("%w: %s", ErrUnavailable, err)
		}
	}
	return nil
}
