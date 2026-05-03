package worker

import (
	"context"
	"time"

	"github.com/julianshen/bi/internal/ocr"
)

// Format identifies a target conversion output.
type Format int

const (
	FormatPDF Format = iota
	FormatPNG
	FormatMarkdown
)

func (f Format) String() string {
	switch f {
	case FormatPDF:
		return "pdf"
	case FormatPNG:
		return "png"
	case FormatMarkdown:
		return "markdown"
	default:
		return "unknown"
	}
}

// MarkdownImageMode controls how embedded images are emitted in Markdown.
type MarkdownImageMode int

const (
	MarkdownImagesEmbed MarkdownImageMode = iota // inline as data: URIs
	MarkdownImagesDrop                           // strip entirely
)

func (m MarkdownImageMode) String() string {
	switch m {
	case MarkdownImagesEmbed:
		return "embed"
	case MarkdownImagesDrop:
		return "drop"
	default:
		return "unknown"
	}
}

// OCRMode controls when OCR runs on PDF pages in the markdown pipeline.
type OCRMode int

const (
	// OCRAuto runs OCR on pages whose extracted text is below the
	// configured threshold. Default zero value.
	OCRAuto OCRMode = iota
	// OCRAlways forces OCR on every page, ignoring the text layer.
	OCRAlways
	// OCRNever disables OCR even on text-layer-empty pages.
	OCRNever
)

func (m OCRMode) String() string {
	switch m {
	case OCRAuto:
		return "auto"
	case OCRAlways:
		return "always"
	case OCRNever:
		return "never"
	default:
		return "unknown"
	}
}

// Job is a single conversion request, fully self-described.
type Job struct {
	InPath         string // path to a temp file already on disk
	Format         Format
	Page           int               // 0-based; PNG only
	Pages          []int             // 0-based selected pages; PNG only; empty means Page
	GridCols       int               // PNG multi-page layout columns; 0 means default
	GridRows       int               // PNG multi-page layout rows; 0 means default
	DPI            float64           // PNG only
	Password       string            // empty if not encrypted
	MarkdownImages MarkdownImageMode // markdown only
	MarkdownMarp   bool              // markdown only; emit Marp front-matter
	// OCR controls (markdown PDF route only). Zero values mean
	// "auto" mode and "auto" language detection.
	OCRMode OCRMode
	OCRLang string
}

// Result describes the output of a successful conversion.
type Result struct {
	OutPath    string // worker-owned temp file; caller os.Removes after streaming
	TotalPages int    // populated for PNG and PDF; 0 for markdown
	MIME       string
}

// Converter is the only surface internal/server depends on.
type Converter interface {
	Run(ctx context.Context, job Job) (Result, error)
}

// Config drives Pool.New. Distinct from internal/config.Config.
type Config struct {
	LOKPath        string
	Workers        int
	QueueDepth     int
	ConvertTimeout time.Duration
	Inst           Instrumenter // optional; nil means no metrics

	// OCR is the optional engine used by the markdown PDF route.
	// Nil disables OCR entirely; OCRMode=auto/always with a nil
	// engine is rejected at the HTTP layer before reaching the
	// worker, so the worker treats nil as "feature disabled".
	OCR ocr.Engine

	// OCRTextThreshold is the per-page extracted-character count
	// below which OCR runs (in OCRAuto mode). Default 16 set by
	// the caller (cmd/bi/convert.go).
	OCRTextThreshold int

	// OCRDPI is the DPI passed to lokDoc.RenderPagePNG when an
	// OCR-needing page is rasterised. Default 300 set by caller.
	OCRDPI float64
}
