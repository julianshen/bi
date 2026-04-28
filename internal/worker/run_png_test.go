package worker

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestPoolRunPNGHappyPath(t *testing.T) {
	pngBytes := []byte{0x89, 0x50, 0x4e, 0x47, 0x00}
	office := &fakeOffice{loadDoc: &fakeDocument{parts: 12, renderBytes: pngBytes}}
	p, _ := newWithOffice(Config{Workers: 1, QueueDepth: 1, ConvertTimeout: time.Second}, office)
	t.Cleanup(func() { _ = p.Close() })

	in := tmpFile(t, "deck.pptx", []byte("x"))
	res, err := p.Run(context.Background(), Job{InPath: in, Format: FormatPNG, Page: 3, DPI: 1.5})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(res.OutPath) })

	if res.MIME != "image/png" {
		t.Errorf("MIME = %q, want image/png", res.MIME)
	}
	if res.TotalPages != 12 {
		t.Errorf("TotalPages = %d, want 12", res.TotalPages)
	}
	got, err := os.ReadFile(res.OutPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, pngBytes) {
		t.Errorf("file bytes = %x, want %x", got, pngBytes)
	}
}

func TestPoolRunPNGRejectsOutOfRangePage(t *testing.T) {
	office := &fakeOffice{loadDoc: &fakeDocument{parts: 5}}
	p, _ := newWithOffice(Config{Workers: 1, QueueDepth: 1, ConvertTimeout: time.Second}, office)
	t.Cleanup(func() { _ = p.Close() })

	in := tmpFile(t, "deck.pptx", []byte("x"))
	_, err := p.Run(context.Background(), Job{InPath: in, Format: FormatPNG, Page: 5, DPI: 1.0})
	if !errors.Is(err, ErrPageOutOfRange) {
		t.Fatalf("err = %v, want ErrPageOutOfRange", err)
	}
}

func TestPoolRunPNGRejectsBadDPI(t *testing.T) {
	office := &fakeOffice{loadDoc: &fakeDocument{parts: 5}}
	p, _ := newWithOffice(Config{Workers: 1, QueueDepth: 1, ConvertTimeout: time.Second}, office)
	t.Cleanup(func() { _ = p.Close() })

	in := tmpFile(t, "deck.pptx", []byte("x"))
	for _, dpi := range []float64{0, -1, 0.05, 5.0} {
		_, err := p.Run(context.Background(), Job{InPath: in, Format: FormatPNG, Page: 0, DPI: dpi})
		if !errors.Is(err, ErrInvalidDPI) {
			t.Errorf("dpi=%v: err = %v, want ErrInvalidDPI", dpi, err)
		}
	}
}
