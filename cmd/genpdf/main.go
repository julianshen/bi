// Command genpdf produces a minimal PDF fixture used by integration
// tests. Run from repo root: `go run ./cmd/genpdf`. The output
// (testdata/health.pdf) is a committed binary; regenerate only when
// fixture text needs to change.
package main

import (
	"log"

	"github.com/jung-kurt/gofpdf"
)

func main() {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 16)
	pdf.Cell(40, 10, "Hello PDF")
	if err := pdf.OutputFileAndClose("testdata/health.pdf"); err != nil {
		log.Fatal(err)
	}
}
