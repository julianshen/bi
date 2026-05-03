package main

import (
	"slices"
	"testing"

	"github.com/julianshen/bi/internal/worker"
)

func TestBuildJobPNGPagesAndLayout(t *testing.T) {
	job, err := buildJob("in.pptx", "png", 0, "0,2,4", "2x2", 1.5, "", "embed", false, "auto", "auto")
	if err != nil {
		t.Fatalf("buildJob: %v", err)
	}
	if job.Format != worker.FormatPNG {
		t.Fatalf("Format = %v, want FormatPNG", job.Format)
	}
	if got, want := job.Pages, []int{0, 2, 4}; !slices.Equal(got, want) {
		t.Fatalf("Pages = %v, want %v", got, want)
	}
	if job.GridCols != 2 || job.GridRows != 2 {
		t.Fatalf("layout = %dx%d, want 2x2", job.GridCols, job.GridRows)
	}
	if job.DPI != 1.5 {
		t.Fatalf("DPI = %v, want 1.5", job.DPI)
	}
}

func TestBuildJobPNGRejectsLayoutWithoutPages(t *testing.T) {
	_, err := buildJob("in.pptx", "png", 0, "", "2x2", 1.0, "", "embed", false, "auto", "auto")
	if err == nil {
		t.Fatal("want error")
	}
}
