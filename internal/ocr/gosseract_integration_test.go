//go:build integration && !noocr

package ocr

import (
	"context"
	"os"
	"strings"
	"testing"
)

func tessdataDirOrSkip(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("TESSDATA_PREFIX")
	if dir == "" {
		t.Skip("TESSDATA_PREFIX not set")
	}
	return dir
}

func TestGosseractRecognizeEnglish(t *testing.T) {
	dir := tessdataDirOrSkip(t)
	eng, err := New(Config{TessdataPath: dir, Languages: SupportedLangs, DPI: 300})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close() })

	img, err := os.ReadFile("testdata/eng.png")
	if err != nil {
		t.Fatal(err)
	}
	got, err := eng.Recognize(context.Background(), img, "eng")
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}
	if !strings.Contains(got, "Hello") {
		t.Errorf("recognised %q does not contain 'Hello'", got)
	}
}

func TestGosseractRecognizeCJK(t *testing.T) {
	dir := tessdataDirOrSkip(t)
	eng, err := New(Config{TessdataPath: dir, Languages: SupportedLangs, DPI: 300})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close() })

	cases := []struct {
		fixture, lang, want string
	}{
		{"testdata/jpn.png", "jpn", "こんにちは"},
		{"testdata/chi_sim.png", "chi_sim", "你好"},
		{"testdata/chi_tra.png", "chi_tra", "您好"},
	}
	for _, c := range cases {
		t.Run(c.lang, func(t *testing.T) {
			img, err := os.ReadFile(c.fixture)
			if err != nil {
				t.Fatal(err)
			}
			got, err := eng.Recognize(context.Background(), img, c.lang)
			if err != nil {
				t.Fatalf("Recognize: %v", err)
			}
			if !strings.Contains(got, c.want) {
				t.Errorf("lang=%s: recognised %q does not contain %q", c.lang, got, c.want)
			}
		})
	}
}

// TestGosseractAutoLang exercises the OSD-driven detection path.
// The Japanese fixture should be detected as Japanese script and
// produce recognisable output.
func TestGosseractAutoLang(t *testing.T) {
	dir := tessdataDirOrSkip(t)
	eng, err := New(Config{TessdataPath: dir, Languages: SupportedLangs, DPI: 300})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close() })

	img, err := os.ReadFile("testdata/jpn.png")
	if err != nil {
		t.Fatal(err)
	}
	got, err := eng.Recognize(context.Background(), img, "")
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}
	// Tesseract may insert spaces between glyphs; strip whitespace
	// before checking. The point is that OSD picked Japanese.
	stripped := strings.Join(strings.Fields(got), "")
	if !strings.Contains(stripped, "こんにちは") {
		t.Errorf("auto recognised %q does not contain 'こんにちは'", got)
	}
}
