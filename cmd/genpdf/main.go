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
	writeImagePDF("testdata/health-scanned.pdf",
		[]string{"internal/ocr/testdata/eng.png"})
	writeImagePDF("testdata/scanned-multi-lang.pdf", []string{
		"internal/ocr/testdata/eng.png",
		"internal/ocr/testdata/jpn.png",
		"internal/ocr/testdata/chi_sim.png",
		"internal/ocr/testdata/chi_tra.png",
	})
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

// writeImagePDF builds an image-only PDF — one PNG per page, no
// embedded text layer. This synthesises a "scanned" PDF for the
// markdown OCR integration path: extractPDFPages returns empty
// strings, the OCR pipeline rasterises each page back to PNG, and
// Tesseract recovers the text.
func writeImagePDF(path string, images []string) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	for _, img := range images {
		pdf.AddPage()
		pdf.ImageOptions(img, 10, 10, 190, 0,
			false, gofpdf.ImageOptions{ImageType: "PNG", ReadDpi: false}, 0, "")
	}
	if err := pdf.OutputFileAndClose(path); err != nil {
		log.Fatal(err)
	}
}
