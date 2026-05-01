// Command genpdf produces minimal PDF fixtures used by integration
// tests. Run from repo root: `go run -tags=fixturegen ./cmd/genpdf`.
// Outputs (testdata/health.pdf and testdata/multi-page.pdf) are
// committed binaries; regenerate only when fixture text changes.
//
// Behind the `fixturegen` build tag so the gofpdf dep stays out of
// the production module graph for `go build ./...`.
//
//go:build fixturegen

package main

import (
	"log"

	"github.com/jung-kurt/gofpdf"
)

func main() {
	writeSinglePage("testdata/health.pdf", "Hello PDF")
	writeMultiPage("testdata/multi-page.pdf", []string{"Page One Body", "Page Two Body"})
}

func writeSinglePage(path, text string) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 16)
	pdf.Cell(40, 10, text)
	if err := pdf.OutputFileAndClose(path); err != nil {
		log.Fatal(err)
	}
}

func writeMultiPage(path string, pages []string) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetFont("Helvetica", "", 16)
	for _, body := range pages {
		pdf.AddPage()
		pdf.Cell(40, 10, body)
	}
	if err := pdf.OutputFileAndClose(path); err != nil {
		log.Fatal(err)
	}
}
