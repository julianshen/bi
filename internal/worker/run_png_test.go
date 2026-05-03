package worker

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os"
	"slices"
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

func TestPoolRunPNGComposesSelectedPagesIntoGrid(t *testing.T) {
	page0 := solidPNG(t, 2, 3, color.RGBA{R: 0xff, A: 0xff})
	page2 := solidPNG(t, 2, 3, color.RGBA{G: 0xff, A: 0xff})
	page4 := solidPNG(t, 2, 3, color.RGBA{B: 0xff, A: 0xff})
	doc := &fakeDocument{
		parts: 6,
		renderByPage: map[int][]byte{
			0: page0,
			2: page2,
			4: page4,
		},
	}
	office := &fakeOffice{loadDoc: doc}
	p, _ := newWithOffice(Config{Workers: 1, QueueDepth: 1, ConvertTimeout: time.Second}, office)
	t.Cleanup(func() { _ = p.Close() })

	in := tmpFile(t, "deck.pptx", []byte("x"))
	res, err := p.Run(context.Background(), Job{
		InPath:   in,
		Format:   FormatPNG,
		Pages:    []int{0, 2, 4},
		GridCols: 2,
		GridRows: 2,
		DPI:      1.25,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(res.OutPath) })

	if got, want := doc.renderCalls, []renderCall{{Page: 0, DPI: 1.25}, {Page: 2, DPI: 1.25}, {Page: 4, DPI: 1.25}}; !slices.Equal(got, want) {
		t.Fatalf("render calls = %+v, want %+v", got, want)
	}
	got := decodePNGFile(t, res.OutPath)
	if got.Bounds().Dx() != 4 || got.Bounds().Dy() != 6 {
		t.Fatalf("bounds = %v, want 4x6", got.Bounds())
	}
	assertPixel(t, got, 0, 0, color.RGBA{R: 0xff, A: 0xff})
	assertPixel(t, got, 2, 0, color.RGBA{G: 0xff, A: 0xff})
	assertPixel(t, got, 0, 3, color.RGBA{B: 0xff, A: 0xff})
	assertPixel(t, got, 2, 3, color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})
}

func TestPoolRunPNGRejectsOversizedGrid(t *testing.T) {
	doc := &fakeDocument{
		parts:       1,
		renderBytes: pngWithConfig(t, 100_001, 1_000),
	}
	office := &fakeOffice{loadDoc: doc}
	p, _ := newWithOffice(Config{Workers: 1, QueueDepth: 1, ConvertTimeout: time.Second}, office)
	t.Cleanup(func() { _ = p.Close() })

	in := tmpFile(t, "deck.pptx", []byte("x"))
	_, err := p.Run(context.Background(), Job{
		InPath:   in,
		Format:   FormatPNG,
		Pages:    []int{0},
		GridCols: 1,
		GridRows: 1,
		DPI:      1,
	})
	if !errors.Is(err, ErrPNGGridTooLarge) {
		t.Fatalf("err = %v, want ErrPNGGridTooLarge", err)
	}
}

func solidPNG(t *testing.T, width, height int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func decodePNGFile(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func assertPixel(t *testing.T, img image.Image, x, y int, want color.RGBA) {
	t.Helper()
	got := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
	if got != want {
		t.Fatalf("pixel(%d,%d) = %#v, want %#v", x, y, got, want)
	}
}

func pngWithConfig(t *testing.T, width, height uint32) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	writePNGChunk(&buf, "IHDR", func() []byte {
		data := make([]byte, 13)
		binary.BigEndian.PutUint32(data[0:4], width)
		binary.BigEndian.PutUint32(data[4:8], height)
		data[8] = 8 // bit depth
		data[9] = 2 // truecolor
		return data
	}())
	writePNGChunk(&buf, "IEND", nil)
	return buf.Bytes()
}

func writePNGChunk(buf *bytes.Buffer, typ string, data []byte) {
	_ = binary.Write(buf, binary.BigEndian, uint32(len(data)))
	chunk := append([]byte(typ), data...)
	buf.Write(chunk)
	_ = binary.Write(buf, binary.BigEndian, crc32.ChecksumIEEE(chunk))
}
