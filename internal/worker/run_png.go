package worker

import (
	"context"
	"fmt"
	"os"
)

const (
	minDPI = 0.1
	maxDPI = 4.0
)

func (p *Pool) runPNG(ctx context.Context, job Job) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if job.DPI < minDPI || job.DPI > maxDPI {
		return Result{}, ErrInvalidDPI
	}
	doc, err := p.office.Load(job.InPath, job.Password)
	if err != nil {
		return Result{}, Classify(err)
	}
	defer doc.Close()

	parts, perr := doc.GetParts()
	if perr != nil {
		return Result{}, Classify(perr)
	}
	if job.Page < 0 || job.Page >= parts {
		return Result{}, ErrPageOutOfRange
	}

	if err := doc.InitializeForRendering(""); err != nil {
		return Result{}, Classify(err)
	}
	pngBytes, err := doc.RenderPagePNG(job.Page, job.DPI)
	if err != nil {
		return Result{}, Classify(err)
	}

	out, err := os.CreateTemp("", "bi-*.png")
	if err != nil {
		return Result{}, fmt.Errorf("worker: create temp: %w", err)
	}
	outPath := out.Name()
	if _, err := out.Write(pngBytes); err != nil {
		out.Close()
		_ = os.Remove(outPath)
		return Result{}, fmt.Errorf("worker: write png: %w", err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(outPath)
		return Result{}, fmt.Errorf("worker: close png: %w", err)
	}
	return Result{OutPath: outPath, TotalPages: parts, MIME: "image/png"}, nil
}
