package mdconv_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/julianshen/bi/internal/mdconv"
)

func TestConvertGolden(t *testing.T) {
	cases := []struct {
		name string
		marp bool
	}{
		{"paragraph", false},
		{"headings", false},
		{"table", false},
		{"image-embed", false},
		{"image-drop", false},
		{"lo-noise", false},
		{"marp-simple", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			html := mustRead(t, filepath.Join("testdata", c.name+".html"))
			wantMD := mustRead(t, filepath.Join("testdata", c.name+".md"))
			opts := mdconv.Options{Images: mdconv.ImagesEmbed, Marp: c.marp}
			if c.name == "image-drop" {
				opts.Images = mdconv.ImagesDrop
			}
			if c.name == "image-embed" || c.name == "image-drop" {
				seedSiblingImage(t, "testdata/x.png", "PNGFAKE")
				t.Cleanup(func() { _ = os.Remove("testdata/x.png") })
				gotMD, err := mdconv.ConvertWithBase(html, opts, "testdata")
				if err != nil {
					t.Fatalf("ConvertWithBase: %v", err)
				}
				if normalise(gotMD) != normalise(wantMD) {
					t.Errorf("output mismatch:\n got: %q\nwant: %q", gotMD, wantMD)
				}
				return
			}
			gotMD, err := mdconv.Convert(html, opts)
			if err != nil {
				t.Fatalf("Convert: %v", err)
			}
			if normalise(gotMD) != normalise(wantMD) {
				t.Errorf("output mismatch:\n got: %q\nwant: %q", gotMD, wantMD)
			}
		})
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// normalise trims trailing whitespace per line + final newline so golden
// files don't have to be byte-perfect with the lib's quirks.
func normalise(b []byte) string {
	lines := strings.Split(string(b), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t\r")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

func seedSiblingImage(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
