package mdconv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyImageModeDropsOversizedImage(t *testing.T) {
	dir := t.TempDir()
	bigPath := filepath.Join(dir, "big.png")
	if err := os.WriteFile(bigPath, make([]byte, maxEmbedImageBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	md := []byte("![alt](big.png)")
	out := applyImageMode(md, ImagesEmbed, dir)
	if strings.Contains(string(out), "data:") {
		t.Errorf("oversized image was embedded: %q", out)
	}
}

func TestApplyImageModeDataURIPassesThrough(t *testing.T) {
	md := []byte("![alt](data:image/png;base64,AAA)")
	out := applyImageMode(md, ImagesEmbed, ".")
	if !strings.Contains(string(out), "data:image/png;base64,AAA") {
		t.Errorf("data URI was rewritten: %q", out)
	}
}

func TestApplyImageModeUnknownMode(t *testing.T) {
	// Default branch — invalid mode value returns input unchanged.
	md := []byte("![alt](x.png)")
	out := applyImageMode(md, ImageMode(99), ".")
	if string(out) != string(md) {
		t.Errorf("unknown mode mutated input: %q", out)
	}
}

func TestIsDataURI(t *testing.T) {
	cases := map[string]bool{
		"data:image/png;base64,xyz": true,
		"data:":                     true,
		"http://example.com":        false,
		"":                          false,
		"dat":                       false,
	}
	for s, want := range cases {
		if got := isDataURI(s); got != want {
			t.Errorf("isDataURI(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestResolveImageSrcRejectsTraversal(t *testing.T) {
	cases := []struct {
		name, base, src string
		ok              bool
	}{
		{"sibling file", "/tmp/bi-abc", "x.png", true},
		{"nested file", "/tmp/bi-abc", "img/x.png", true},
		{"parent traversal", "/tmp/bi-abc", "../etc/passwd", false},
		{"deep traversal", "/tmp/bi-abc", "../../etc/passwd", false},
		{"absolute path", "/tmp/bi-abc", "/etc/passwd", false},
		{"sneaky equal-prefix dir", "/tmp/bi", "../bi-other/x", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, ok := resolveImageSrc(c.base, c.src)
			if ok != c.ok {
				t.Fatalf("resolveImageSrc(%q, %q) ok = %v, want %v", c.base, c.src, ok, c.ok)
			}
		})
	}
}
