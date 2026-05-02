package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/julianshen/bi/internal/config"
	"github.com/julianshen/bi/internal/ocr"
	"github.com/julianshen/bi/internal/worker"
)

// runConvert runs a single document conversion via the in-process worker.
// Used by the SubprocessConverter so the HTTP server can isolate cgo/LO
// crashes to a child process — each invocation gets a fresh LO init and
// exits when the conversion is done. Issue #3 documents the underlying
// reason for the subprocess pattern.
func runConvert(args []string) {
	// Reset the signal mask before doing anything cgo-adjacent. (See
	// sigmask_linux.go for context.)
	resetSignalMask()

	fs := flag.NewFlagSet("convert", flag.ExitOnError)
	in := fs.String("in", "", "input file path (required)")
	out := fs.String("out", "", "output file path (required)")
	format := fs.String("format", "", "pdf | png | markdown (required)")
	page := fs.Int("page", 0, "page index (PNG only, 0-based)")
	dpi := fs.Float64("dpi", 1.0, "DPI scale (PNG only)")
	password := fs.String("password", "", "document password (optional)")
	images := fs.String("images", "embed", "embed | drop (markdown only)")
	marp := fs.Bool("marp", false, "emit Marp-style markdown (markdown only)")
	ocrFlag := fs.String("ocr", "auto", "auto | always | never (markdown only)")
	ocrLang := fs.String("ocr-lang", "auto", "OCR language (markdown only)")
	timeout := fs.Duration("timeout", 120*time.Second, "max time for the conversion")
	if err := fs.Parse(args); err != nil {
		failConvert("bad-flags", err)
	}
	if *in == "" || *out == "" || *format == "" {
		failConvert("bad-flags", errors.New("-in, -out, -format are required"))
	}

	job, err := buildJob(*in, *format, *page, *dpi, *password, *images, *marp, *ocrFlag, *ocrLang)
	if err != nil {
		failConvert("bad-flags", err)
	}

	cfg, err := config.Load(envMap())
	if err != nil {
		failConvert("config", err)
	}
	if cfg.LOKPath == "" {
		path, err := config.ResolveLOKPath(config.LOKPathSources{
			Defaults: config.PlatformDefaults(),
		})
		if err != nil {
			failConvert("lok-path", err)
		}
		cfg.LOKPath = path
	}

	// OCR engine setup. Tessdata path resolves from cfg first
	// (BI_OCR_TESSDATA / TESSDATA_PREFIX handled in config.Load),
	// falling back to TESSDATA_PREFIX direct read for older envs.
	tessdata := cfg.OCRTessdataPath
	if tessdata == "" {
		tessdata = os.Getenv("TESSDATA_PREFIX")
	}
	var ocrEngine ocr.Engine
	if cfg.OCREnabled && job.Format == worker.FormatMarkdown && job.OCRMode != worker.OCRNever && tessdata != "" {
		var engineErr error
		ocrEngine, engineErr = ocr.New(ocr.Config{
			TessdataPath: tessdata,
			Languages:    ocr.SupportedLangs,
			DPI:          cfg.OCRDPI,
		})
		if engineErr != nil {
			if job.OCRMode == worker.OCRAlways {
				failConvert("ocr-unavailable", engineErr)
			}
			ocrEngine = nil
		}
		if ocrEngine != nil {
			defer ocrEngine.Close()
		}
	}
	if job.OCRMode == worker.OCRAlways && ocrEngine == nil {
		failConvert("ocr-unavailable", errors.New("OCR engine unavailable"))
	}

	pool, err := worker.New(worker.Config{
		LOKPath:          cfg.LOKPath,
		Workers:          1,
		QueueDepth:       1,
		ConvertTimeout:   *timeout,
		OCR:              ocrEngine,
		OCRTextThreshold: cfg.OCRTextThreshold,
		OCRDPI:           cfg.OCRDPI,
	})
	if err != nil {
		failConvert(classifyConvertErr(err), err)
	}
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	res, err := pool.Run(ctx, job)
	if err != nil {
		failConvert(classifyConvertErr(err), err)
	}
	defer os.Remove(res.OutPath)

	if err := os.Rename(res.OutPath, *out); err != nil {
		// Cross-device rename fails on /tmp → bind-mounted target. Fall back
		// to copy + remove.
		if err := copyFile(res.OutPath, *out); err != nil {
			failConvert("write-output", err)
		}
	}

	// Help GC give cgo a clean shutdown — workers may still hold onto
	// callback dispatcher goroutines.
	runtime.GC()

	enc := json.NewEncoder(os.Stdout)
	_ = enc.Encode(map[string]any{
		"mime":        res.MIME,
		"total_pages": res.TotalPages,
	})
	os.Exit(0)
}

// buildJob translates CLI flags into a worker.Job. Validation that's local
// to this binary lives here; semantic validation (page range, dpi range)
// happens inside worker.Run.
func buildJob(in, format string, page int, dpi float64, password, images string, marp bool, ocrMode, ocrLang string) (worker.Job, error) {
	job := worker.Job{InPath: in, Password: password}
	switch strings.ToLower(format) {
	case "pdf":
		job.Format = worker.FormatPDF
	case "png":
		job.Format = worker.FormatPNG
		job.Page = page
		job.DPI = dpi
	case "markdown", "md":
		job.Format = worker.FormatMarkdown
		switch images {
		case "drop":
			job.MarkdownImages = worker.MarkdownImagesDrop
		case "embed", "":
			job.MarkdownImages = worker.MarkdownImagesEmbed
		default:
			return job, fmt.Errorf("invalid -images value %q", images)
		}
		job.MarkdownMarp = marp
		switch ocrMode {
		case "auto", "":
			job.OCRMode = worker.OCRAuto
		case "always":
			job.OCRMode = worker.OCRAlways
		case "never":
			job.OCRMode = worker.OCRNever
		default:
			return job, fmt.Errorf("invalid -ocr value %q", ocrMode)
		}
		job.OCRLang = ocrLang
	default:
		return job, fmt.Errorf("invalid -format value %q", format)
	}
	return job, nil
}

// classifyConvertErr maps a worker error to a stable string the parent
// process can switch on. The strings here are the contract between
// `bi convert` (this file) and SubprocessConverter (server package).
func classifyConvertErr(err error) string {
	switch {
	case errors.Is(err, worker.ErrPasswordRequired):
		return "password-required"
	case errors.Is(err, worker.ErrWrongPassword):
		return "password-wrong"
	case errors.Is(err, worker.ErrUnsupportedFormat):
		return "unsupported-document"
	case errors.Is(err, worker.ErrLOKUnsupported):
		return "lok-unsupported"
	case errors.Is(err, worker.ErrPageOutOfRange):
		return "page-out-of-range"
	case errors.Is(err, worker.ErrInvalidDPI):
		return "invalid-dpi"
	case errors.Is(err, worker.ErrMarkdownConversion):
		return "markdown-pipeline"
	case errors.Is(err, worker.ErrOCRFailed):
		return "ocr-failed"
	case errors.Is(err, worker.ErrOCRUnavailable):
		return "ocr-unavailable"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	default:
		return "internal"
	}
}

// failConvert prints a JSON error to stdout and exits 1. Stdout (not
// stderr) is the contract because SubprocessConverter parses the last
// stdout line; any LO chatter on stderr is forwarded for ops visibility.
func failConvert(kind string, err error) {
	enc := json.NewEncoder(os.Stdout)
	_ = enc.Encode(map[string]any{"error": kind, "detail": err.Error()})
	os.Exit(1)
}

// copyFile streams src → dst; used when os.Rename can't span filesystems.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := copyBuf(in, out); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func copyBuf(in *os.File, out *os.File) (int64, error) {
	return io.Copy(out, in)
}
