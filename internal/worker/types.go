package worker

import (
	"context"
	"time"
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

// Job is a single conversion request, fully self-described.
type Job struct {
	InPath         string // path to a temp file already on disk
	Format         Format
	Page           int               // 0-based; PNG only
	DPI            float64           // PNG only
	Password       string            // empty if not encrypted
	MarkdownImages MarkdownImageMode // markdown only
	MarkdownMarp   bool              // markdown only; emit Marp front-matter
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
}
