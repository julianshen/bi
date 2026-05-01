package server

import "testing"

func TestExtensionFromContentType(t *testing.T) {
	cases := []struct {
		ct, want string
	}{
		{"application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".docx"},
		{"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".xlsx"},
		{"application/vnd.openxmlformats-officedocument.presentationml.presentation", ".pptx"},
		{"application/vnd.oasis.opendocument.text", ".odt"},
		{"application/vnd.oasis.opendocument.spreadsheet", ".ods"},
		{"application/vnd.oasis.opendocument.presentation", ".odp"},
		{"application/vnd.oasis.opendocument.graphics", ".odg"},
		{"application/msword", ".doc"},
		{"application/vnd.ms-excel", ".xls"},
		{"application/vnd.ms-powerpoint", ".ppt"},
		{"application/pdf", ".pdf"},
		{"application/rtf", ".rtf"},
		{"text/rtf", ".rtf"},
		{"text/plain", ".txt"},
		{"text/html", ".html"},
		{"text/csv", ".csv"},
		{"text/plain; charset=utf-8", ".txt"},
		{"application/octet-stream", ".bin"}, // resolves via mime.ExtensionsByType
		{"image/png", ".png"},                // ditto
		{"unknown/zzz-no-such-subtype", ""},  // unparseable parses fine but maps to nothing
		{"not even a media type;;;", ""},     // genuine parse error
	}
	for _, c := range cases {
		t.Run(c.ct, func(t *testing.T) {
			got := extensionFromContentType(c.ct)
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
