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

	"github.com/julianshen/bi/internal/pngopts"
)

const (
	minDPI                 = 0.1
	maxDPI                 = 4.0
	maxPNGGridPixels int64 = 100_000_000
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
	if len(pages) > pngopts.MaxSelectedPages {
		return Result{}, ErrPNGGridTooLarge
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
	if len(pages) == 1 && job.GridCols <= 0 && job.GridRows <= 0 {
		ctx, span = tracer.Start(ctx, "lok.render_png")
		pngBytes, err := doc.RenderPagePNG(pages[0], job.DPI)
		span.End()
		if err != nil {
			return Result{}, Classify(err)
		}
		return writePNGResult(pngBytes, parts)
	}

	pagePaths := make([]string, 0, len(pages))
	defer func() {
		for _, path := range pagePaths {
			_ = os.Remove(path)
		}
	}()
	for _, page := range pages {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		ctx, span = tracer.Start(ctx, "lok.render_png")
		pngBytes, err := doc.RenderPagePNG(page, job.DPI)
		span.End()
		if err != nil {
			return Result{}, Classify(err)
		}
		path, err := writeTempPNG("bi-page-*.png", pngBytes)
		if err != nil {
			return Result{}, err
		}
		pagePaths = append(pagePaths, path)
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	pngBytes, err := composePNGGrid(ctx, pagePaths, job.GridCols, job.GridRows)
	if err != nil {
		return Result{}, err
	}
	return writePNGResult(pngBytes, parts)
}

func writePNGResult(pngBytes []byte, parts int) (Result, error) {
	outPath, err := writeTempPNG("bi-*.png", pngBytes)
	if err != nil {
		return Result{}, err
	}
	return Result{OutPath: outPath, TotalPages: parts, MIME: "image/png"}, nil
}

func writeTempPNG(pattern string, pngBytes []byte) (string, error) {
	out, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", fmt.Errorf("worker: create temp: %w", err)
	}
	outPath := out.Name()
	if _, err := out.Write(pngBytes); err != nil {
		out.Close()
		_ = os.Remove(outPath)
		return "", fmt.Errorf("worker: write png: %w", err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(outPath)
		return "", fmt.Errorf("worker: close png: %w", err)
	}
	return outPath, nil
}

func composePNGGrid(ctx context.Context, pagePaths []string, cols, rows int) ([]byte, error) {
	if cols <= 0 {
		cols = len(pagePaths)
	}
	if rows <= 0 {
		rows = (len(pagePaths) + cols - 1) / cols
	}
	if cols <= 0 || rows <= 0 || int64(len(pagePaths)) > int64(cols)*int64(rows) {
		return nil, ErrPageOutOfRange
	}
	if int64(cols)*int64(rows) > pngopts.MaxGridCells {
		return nil, ErrPNGGridTooLarge
	}

	cellWidth, cellHeight := 0, 0
	for _, path := range pagePaths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		cfg, err := decodePNGConfig(path)
		if err != nil {
			return nil, err
		}
		if cfg.Width > cellWidth {
			cellWidth = cfg.Width
		}
		if cfg.Height > cellHeight {
			cellHeight = cfg.Height
		}
	}
	if int64(cols)*int64(cellWidth)*int64(rows)*int64(cellHeight) > maxPNGGridPixels {
		return nil, ErrPNGGridTooLarge
	}

	canvas := image.NewRGBA(image.Rect(0, 0, cols*cellWidth, rows*cellHeight))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	for i, path := range pagePaths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		img, err := decodePNGImage(path)
		if err != nil {
			return nil, err
		}
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

func decodePNGConfig(path string) (image.Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return image.Config{}, fmt.Errorf("worker: open rendered png: %w", err)
	}
	defer f.Close()
	cfg, err := png.DecodeConfig(f)
	if err != nil {
		return image.Config{}, fmt.Errorf("worker: decode png config: %w", err)
	}
	return cfg, nil
}

func decodePNGImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("worker: open rendered png: %w", err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("worker: decode rendered png: %w", err)
	}
	return img, nil
}
