package worker

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
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
	ctx, span := tracer.Start(ctx, "lok.load")
	doc, err := p.office.Load(job.InPath, job.Password)
	span.End()
	if err != nil {
		return Result{}, Classify(err)
	}
	defer doc.Close()

	parts, perr := doc.GetParts()
	if perr != nil {
		return Result{}, Classify(perr)
	}
	pages := job.Pages
	if len(pages) == 0 {
		pages = []int{job.Page}
	}
	for _, page := range pages {
		if page < 0 || page >= parts {
			return Result{}, ErrPageOutOfRange
		}
	}

	ctx, span = tracer.Start(ctx, "lok.save_as")
	err = doc.InitializeForRendering("")
	span.End()
	if err != nil {
		return Result{}, Classify(err)
	}
	rendered := make([][]byte, 0, len(pages))
	for _, page := range pages {
		ctx, span = tracer.Start(ctx, "lok.render_png")
		pngBytes, err := doc.RenderPagePNG(page, job.DPI)
		span.End()
		if err != nil {
			return Result{}, Classify(err)
		}
		rendered = append(rendered, pngBytes)
	}
	pngBytes := rendered[0]
	if len(rendered) > 1 || job.GridCols > 0 || job.GridRows > 0 {
		pngBytes, err = composePNGGrid(rendered, job.GridCols, job.GridRows)
		if err != nil {
			return Result{}, err
		}
	}
	return writePNGResult(pngBytes, parts)
}

func writePNGResult(pngBytes []byte, parts int) (Result, error) {
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

func composePNGGrid(pages [][]byte, cols, rows int) ([]byte, error) {
	if cols <= 0 {
		cols = len(pages)
	}
	if rows <= 0 {
		rows = (len(pages) + cols - 1) / cols
	}
	if cols <= 0 || rows <= 0 || len(pages) > cols*rows {
		return nil, ErrPageOutOfRange
	}

	decoded := make([]image.Image, 0, len(pages))
	cellWidth, cellHeight := 0, 0
	for _, page := range pages {
		img, err := png.Decode(bytes.NewReader(page))
		if err != nil {
			return nil, fmt.Errorf("worker: decode rendered png: %w", err)
		}
		decoded = append(decoded, img)
		bounds := img.Bounds()
		if bounds.Dx() > cellWidth {
			cellWidth = bounds.Dx()
		}
		if bounds.Dy() > cellHeight {
			cellHeight = bounds.Dy()
		}
	}

	canvas := image.NewRGBA(image.Rect(0, 0, cols*cellWidth, rows*cellHeight))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	for i, img := range decoded {
		x := (i % cols) * cellWidth
		y := (i / cols) * cellHeight
		dst := image.Rect(x, y, x+img.Bounds().Dx(), y+img.Bounds().Dy())
		draw.Draw(canvas, dst, img, img.Bounds().Min, draw.Src)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, canvas); err != nil {
		return nil, fmt.Errorf("worker: encode png grid: %w", err)
	}
	return buf.Bytes(), nil
}
