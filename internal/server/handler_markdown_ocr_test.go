// internal/server/handler_markdown_ocr_test.go
package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/julianshen/bi/internal/server"
	"github.com/julianshen/bi/internal/worker"
)

func TestMarkdownHandlerParsesOCRParams(t *testing.T) {
	conv := &fakeConverter{body: []byte("# heading"), mime: "text/markdown"}
	h := server.New(server.Deps{Conv: conv, MaxUploadBytes: 1 << 20, OCRAvailable: true})
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/convert/markdown?ocr=always&ocr_lang=jpn", strings.NewReader("%PDF-1.4\n%fake"))
	req.Header.Set("Content-Type", "application/pdf")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if conv.got.OCRMode != worker.OCRAlways {
		t.Errorf("OCRMode = %v, want OCRAlways", conv.got.OCRMode)
	}
	if conv.got.OCRLang != "jpn" {
		t.Errorf("OCRLang = %q, want jpn", conv.got.OCRLang)
	}
}

func TestMarkdownHandlerRejectsBadOCR(t *testing.T) {
	conv := &fakeConverter{body: []byte("# heading"), mime: "text/markdown"}
	h := server.New(server.Deps{Conv: conv, MaxUploadBytes: 1 << 20, OCRAvailable: true})
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	for _, q := range []string{"ocr=maybe", "ocr_lang=fra", "ocr_lang=eng+fra", "ocr_lang=ENG"} {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/convert/markdown?"+q, strings.NewReader("%PDF-1.4"))
		req.Header.Set("Content-Type", "application/pdf")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("query %q: status = %d, want 400", q, resp.StatusCode)
		}
	}
}

func TestMarkdownHandler503WhenOCRUnavailable(t *testing.T) {
	conv := &fakeConverter{body: []byte("# heading"), mime: "text/markdown"}
	h := server.New(server.Deps{Conv: conv, MaxUploadBytes: 1 << 20, OCRAvailable: false})
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/convert/markdown?ocr=always", strings.NewReader("%PDF-1.4"))
	req.Header.Set("Content-Type", "application/pdf")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", resp.StatusCode)
	}
}
