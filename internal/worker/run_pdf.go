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
	ctx, span := tracer.Start(ctx, "lok.load")
	doc, err := p.office.Load(job.InPath, job.Password)
	span.End()
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
	ctx, span = tracer.Start(ctx, "lok.save_as")
	err = doc.SaveAs(outPath, "pdf", "")
	span.End()
	if err != nil {
		_ = os.Remove(outPath)
		return Result{}, Classify(err)
	}
	return Result{
		OutPath:    outPath,
		TotalPages: parts,
		MIME:       "application/pdf",
	}, nil
}
