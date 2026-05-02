package worker_test

import (
	"testing"

	"github.com/julianshen/bi/internal/worker"
)

func TestFormatString(t *testing.T) {
	cases := map[worker.Format]string{
		worker.FormatPDF:      "pdf",
		worker.FormatPNG:      "png",
		worker.FormatMarkdown: "markdown",
		worker.Format(99):     "unknown",
	}
	for f, want := range cases {
		if got := f.String(); got != want {
			t.Errorf("Format(%d).String() = %q, want %q", f, got, want)
		}
	}
}

func TestMarkdownImageModeString(t *testing.T) {
	cases := map[worker.MarkdownImageMode]string{
		worker.MarkdownImagesEmbed:  "embed",
		worker.MarkdownImagesDrop:   "drop",
		worker.MarkdownImageMode(9): "unknown",
	}
	for m, want := range cases {
		if got := m.String(); got != want {
			t.Errorf("MarkdownImageMode(%d).String() = %q, want %q", m, got, want)
		}
	}
}

func TestOCRModeString(t *testing.T) {
	cases := map[worker.OCRMode]string{
		worker.OCRAuto:     "auto",
		worker.OCRAlways:   "always",
		worker.OCRNever:    "never",
		worker.OCRMode(99): "unknown",
	}
	for m, want := range cases {
		if got := m.String(); got != want {
			t.Errorf("OCRMode(%d).String() = %q, want %q", m, got, want)
		}
	}
}

func TestJobZeroValueOCR(t *testing.T) {
	var j worker.Job
	if j.OCRMode != worker.OCRAuto {
		t.Errorf("default Job.OCRMode = %v, want OCRAuto (zero value)", j.OCRMode)
	}
	if j.OCRLang != "" {
		t.Errorf("default Job.OCRLang = %q, want empty", j.OCRLang)
	}
}
