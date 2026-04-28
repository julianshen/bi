package mdconv_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/julianshen/bi/internal/mdconv"
)

func TestConvertGolden(t *testing.T) {
	cases := []string{"paragraph"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			html := mustRead(t, filepath.Join("testdata", name+".html"))
			wantMD := mustRead(t, filepath.Join("testdata", name+".md"))
			gotMD, err := mdconv.Convert(html, mdconv.Options{Images: mdconv.ImagesEmbed})
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
