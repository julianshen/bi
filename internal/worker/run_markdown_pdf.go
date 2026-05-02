package worker

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ledongthuc/pdf"
)

// extractPDFPages returns one string per PDF page, in order. Empty
// pages produce an empty string at the corresponding index.
//
// This is the per-page form used by the OCR-aware markdown pipeline.
// extractPDFText is preserved for the existing whole-document tests.
func extractPDFPages(path string) ([]string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		if isEncryptedPDFErr(err) {
			return nil, fmt.Errorf("%w: encrypted PDFs are not supported on the markdown route", ErrPasswordRequired)
		}
		return nil, fmt.Errorf("worker: open pdf: %w", err)
	}
	defer f.Close()

	out := make([]string, 0, r.NumPage())
	for i := 1; i <= r.NumPage(); i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			out = append(out, "")
			continue
		}
		rows, err := page.GetTextByRow()
		if err != nil {
			return nil, fmt.Errorf("worker: pdf page %d: %w", i, err)
		}
		var sb strings.Builder
		for _, row := range rows {
			for j, w := range row.Content {
				if j > 0 {
					sb.WriteByte(' ')
				}
				sb.WriteString(w.S)
			}
			sb.WriteByte('\n')
		}
		out = append(out, sb.String())
	}
	return out, nil
}

// extractPDFText extracts visible text from a PDF using a Go-native
// reader. LibreOffice's pdfimport flattens PDF pages to embedded
// images on load, so the existing pdf → html → markdown path cannot
// recover text.
//
// Output is plain text: one line per visual row, blank line between
// pages. Scanned/image-only PDFs surface as empty output (no OCR);
// callers that send markdown metacharacters in PDF body text get them
// passed through unescaped — text/markdown here is "best-effort plain
// text", not a strict CommonMark guarantee.
//
// Encrypted PDFs are not supported on this path: ledongthuc/pdf does
// not accept a password, so job.Password (which works on the LO path
// for office docs) is ignored here. We detect encryption from the
// library's error string and surface ErrPasswordRequired so clients
// get a 422 instead of an opaque 5xx.
func extractPDFText(path string) ([]byte, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		if isEncryptedPDFErr(err) {
			return nil, fmt.Errorf("%w: encrypted PDFs are not supported on the markdown route", ErrPasswordRequired)
		}
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

// isPDFInput reports whether the staged input file is a PDF. Inputs
// are staged with an extension matching their declared Content-Type;
// .pdf indicates application/pdf. Case-insensitive guard against
// non-normalising stagers.
func isPDFInput(inPath string) bool {
	return strings.EqualFold(filepath.Ext(inPath), ".pdf")
}

// isEncryptedPDFErr matches the substrings ledongthuc/pdf surfaces
// when opening encrypted documents. The library does not expose a
// typed error for this case; pin the strings the upstream emits as of
// v0.0.0-20250511 so the test catches a wording change.
func isEncryptedPDFErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "encrypt") || strings.Contains(msg, "password")
}

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
