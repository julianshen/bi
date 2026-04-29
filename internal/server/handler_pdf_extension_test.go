package server

import "testing"

func TestExtensionFromContentType(t *testing.T) {
	cases := map[string]string{
		// Office (OOXML)
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   ".docx",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         ".xlsx",
		"application/vnd.openxmlformats-officedocument.presentationml.presentation": ".pptx",
		// OpenDocument
		"application/vnd.oasis.opendocument.text":         ".odt",
		"application/vnd.oasis.opendocument.spreadsheet":  ".ods",
		"application/vnd.oasis.opendocument.presentation": ".odp",
		"application/vnd.oasis.opendocument.graphics":     ".odg",
		// Legacy MS Office
		"application/msword":            ".doc",
		"application/vnd.ms-excel":      ".xls",
		"application/vnd.ms-powerpoint": ".ppt",
		// Text-ish
		"application/rtf": ".rtf",
		"text/rtf":        ".rtf",
		"text/plain":      ".txt",
		"text/html":       ".html",
		"text/csv":        ".csv",
		// With charset parameter (mime.ParseMediaType handles)
		"text/plain; charset=utf-8": ".txt",
		// Generic application fallback
		"application/octet-stream": ".bin",
		// Unknown
		"weird/garbage": "",
	}
	for ct, want := range cases {
		got := extensionFromContentType(ct)
		if got != want {
			t.Errorf("extensionFromContentType(%q) = %q, want %q", ct, got, want)
		}
	}
}

func TestExtensionFromContentTypeUnparseable(t *testing.T) {
	// Truly malformed media type — ParseMediaType returns err.
	got := extensionFromContentType("not even a media type;;;")
	if got != "" {
		t.Errorf("got %q, want empty for unparseable", got)
	}
}
