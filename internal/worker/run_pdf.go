package worker

import (
	"context"
	"fmt"
	"os"
)

func (p *Pool) runPDF(ctx context.Context, job Job) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	doc, err := p.office.Load(job.InPath, job.Password)
	if err != nil {
		return Result{}, Classify(err)
	}
	defer doc.Close()

	out, err := os.CreateTemp("", "bi-*.pdf")
	if err != nil {
		return Result{}, fmt.Errorf("worker: create temp: %w", err)
	}
	outPath := out.Name()
	out.Close()

	parts, perr := doc.GetParts()
	if perr != nil {
		_ = os.Remove(outPath)
		return Result{}, Classify(perr)
	}
	if err := doc.SaveAs(outPath, "pdf", ""); err != nil {
		_ = os.Remove(outPath)
		return Result{}, Classify(err)
	}
	return Result{
		OutPath:    outPath,
		TotalPages: parts,
		MIME:       "application/pdf",
	}, nil
}
