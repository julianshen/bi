package worker

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/ledongthuc/pdf"
)

// extractPDFText extracts visible text from a PDF using a Go-native
// reader. LibreOffice's pdfimport flattens PDF pages to images so the
// existing pdf → html → markdown path cannot recover text — see the
// design spec at docs/superpowers/specs/2026-05-01-pdf-input-design.md.
//
// The output is plain text with one line per visual row and a blank
// line between pages. That is already valid markdown (paragraphs
// separated by blanks). Scanned/image-only PDFs surface as empty
// output, matching the documented limitation.
func extractPDFText(path string) ([]byte, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("worker: open pdf: %w", err)
	}
	defer f.Close()

	var buf bytes.Buffer
	for i := 1; i <= r.NumPage(); i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		rows, err := page.GetTextByRow()
		if err != nil {
			return nil, fmt.Errorf("worker: pdf page %d: %w", i, err)
		}
		if i > 1 {
			buf.WriteByte('\n')
		}
		for _, row := range rows {
			for j, w := range row.Content {
				if j > 0 {
					buf.WriteByte(' ')
				}
				buf.WriteString(w.S)
			}
			buf.WriteByte('\n')
		}
	}
	return bytes.TrimSpace(buf.Bytes()), nil
}

// isPDFInput reports whether the staged input file is a PDF. The
// handler stages uploads with extensions chosen by extensionFromContentType;
// .pdf is the contract for application/pdf bodies.
func isPDFInput(inPath string) bool {
	return strings.EqualFold(filepathExt(inPath), ".pdf")
}

// filepathExt is filepath.Ext inlined so this file has one fewer
// import line. Matches stdlib behaviour byte-for-byte for our case.
func filepathExt(path string) string {
	for i := len(path) - 1; i >= 0 && path[i] != '/'; i-- {
		if path[i] == '.' {
			return path[i:]
		}
	}
	return ""
}

// writePDFMarkdownResult is the shared tail used by runMarkdown when
// the input is a PDF. Kept here so run_markdown.go stays focused on
// the LO pipeline.
func writePDFMarkdownResult(text []byte) (Result, error) {
	out, err := os.CreateTemp("", "bi-*.md")
	if err != nil {
		return Result{}, fmt.Errorf("worker: create md temp: %w", err)
	}
	outPath := out.Name()
	if _, err := out.Write(text); err != nil {
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
