//go:build !noocr

package ocr

// CGo build note (macOS / Homebrew arm64):
// gosseract's preprocessflags_x.go hardcodes -I/usr/local/include which does
// not exist on Apple Silicon Homebrew installs. If the build fails with
// "'leptonica/allheaders.h' file not found", use CGO_CPPFLAGS (not CGO_CFLAGS)
// because tessbridge.cpp is compiled as C++ and CPPFLAGS covers both:
//
//   CGO_CPPFLAGS="-I/opt/homebrew/Cellar/tesseract/5.5.2/include \
//                 -I/opt/homebrew/Cellar/leptonica/1.87.0/include" \
//   CGO_LDFLAGS="-L/opt/homebrew/Cellar/tesseract/5.5.2/lib \
//               -L/opt/homebrew/Cellar/leptonica/1.87.0/lib \
//               -ltesseract -lleptonica"
//
// On Linux (Docker / CI) with a system-wide Tesseract install (apt/system
// package), headers and libraries are in the default search paths and no
// extra env vars are required.

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/otiai10/gosseract/v2"
)

// New constructs a gosseract-backed Engine. The tessdata directory
// and language packs are validated at construction time so that
// startup failures surface at boot, not under load.
func New(cfg Config) (Engine, error) {
	if err := Probe(cfg.TessdataPath, cfg.Languages); err != nil {
		return nil, err
	}
	return &gosseractEngine{
		tessdata: cfg.TessdataPath,
		langs:    append([]string(nil), cfg.Languages...),
	}, nil
}

type gosseractEngine struct {
	tessdata string
	langs    []string
	mu       sync.Mutex // gosseract.Client is not safe for concurrent use
}

// Recognize creates a fresh gosseract.Client per call. We intentionally
// do not reuse a client across calls: the cgo state is global, and the
// per-page fork-cost is dominated by Tesseract recognition itself.
func (e *gosseractEngine) Recognize(ctx context.Context, image []byte, langs string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	c := gosseract.NewClient()
	defer c.Close()

	if err := c.SetTessdataPrefix(e.tessdata); err != nil {
		return "", fmt.Errorf("ocr: set tessdata: %w", err)
	}

	if langs == "" {
		// OSD path: detect script first, then re-recognize with the
		// mapped language. Falls back to all-langs-in-one-pass when
		// OSD is inconclusive (short or noisy pages).
		detected, err := detectScript(c, e.tessdata, image)
		if err != nil {
			return "", err
		}
		mapped := scriptToLang(detected)
		if mapped == "" {
			mapped = strings.Join(SupportedLangs, "+")
		}
		langs = mapped
	}

	// Re-create the client after OSD: SetTessdataPrefix + SetLanguage
	// together flag for re-init, so the second Init call picks up the
	// new language without carrying over OSD PSM state.
	c2 := gosseract.NewClient()
	defer c2.Close()

	if err := c2.SetTessdataPrefix(e.tessdata); err != nil {
		return "", fmt.Errorf("ocr: set tessdata (pass2): %w", err)
	}
	if err := c2.SetLanguage(strings.Split(langs, "+")...); err != nil {
		return "", fmt.Errorf("ocr: set language %q: %w", langs, err)
	}
	if err := c2.SetImageFromBytes(image); err != nil {
		return "", fmt.Errorf("ocr: set image: %w", err)
	}
	text, err := c2.Text()
	if err != nil {
		return "", fmt.Errorf("ocr: recognize: %w", err)
	}
	return text, nil
}

func (e *gosseractEngine) Close() error { return nil }

// detectScript runs Tesseract's Orientation and Script Detection on
// the image and returns the detected script name (e.g. "Latin",
// "Japanese", "HanS", "HanT"). Empty string means OSD was inconclusive.
//
// gosseract API note: we use SetPageSegMode(PSM_OSD_ONLY) rather than
// SetVariable("tessedit_pageseg_mode", "0") because gosseract v2.4.1
// exposes a typed SetPageSegMode method — SetVariable requires a
// SettableVariable type and "tessedit_pageseg_mode" is not in the
// SettableVariable allowlist exposed by the package.
func detectScript(c *gosseract.Client, tessdata string, image []byte) (string, error) {
	if err := c.SetLanguage("osd"); err != nil {
		return "", fmt.Errorf("ocr: set osd: %w", err)
	}
	if err := c.SetPageSegMode(gosseract.PSM_OSD_ONLY); err != nil {
		return "", fmt.Errorf("ocr: set psm OSD_ONLY: %w", err)
	}
	if err := c.SetImageFromBytes(image); err != nil {
		return "", fmt.Errorf("ocr: osd set image: %w", err)
	}
	out, err := c.Text()
	if err != nil {
		// OSD failure is non-fatal; caller falls back to all-langs.
		return "", nil
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "Script: ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Script: ")), nil
		}
	}
	return "", nil
}

// scriptToLang maps Tesseract OSD script names to the language code
// the project's tessdata install ships. Unknown scripts return "".
func scriptToLang(script string) string {
	switch script {
	case "Latin", "Cyrillic", "Greek":
		return "eng"
	case "Japanese":
		return "jpn"
	case "HanS":
		return "chi_sim"
	case "HanT":
		return "chi_tra"
	default:
		return ""
	}
}
