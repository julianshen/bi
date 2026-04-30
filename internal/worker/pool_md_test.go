package worker

import "testing"

func TestMDAdapterDelegates(t *testing.T) {
	got, err := mdAdapter{}.Convert([]byte("<p>hi</p>"), MarkdownImagesEmbed, ".", false)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) == "" {
		t.Fatal("empty output")
	}
}

func TestMDAdapterMapsDropMode(t *testing.T) {
	// With Drop mode, an image-bearing HTML should produce output without
	// a Markdown image reference.
	got, err := mdAdapter{}.Convert([]byte(`<p>x</p><img src="missing.png" alt="a"/><p>y</p>`), MarkdownImagesDrop, ".", false)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) == "" {
		t.Fatal("empty output")
	}
	if !contains(got, []byte("x")) || !contains(got, []byte("y")) {
		t.Errorf("missing text content: %q", got)
	}
	if contains(got, []byte("![")) {
		t.Errorf("Drop mode should strip image: %q", got)
	}
}

func contains(haystack, needle []byte) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
