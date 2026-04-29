package mdconv

import "testing"

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
