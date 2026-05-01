package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/julianshen/bi/internal/server"
	"github.com/julianshen/bi/internal/worker"
)

func TestPNGHandlerHappyPath(t *testing.T) {
	conv := &fakeConverter{body: []byte("PNG"), mime: "image/png", pages: 12}
	h := server.New(server.Deps{Conv: conv, MaxUploadBytes: 1 << 20})
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest("POST", srv.URL+"/v1/convert/png?page=4&dpi=1.5", strings.NewReader("x"))
	req.Header.Set("Content-Type", "application/x-test")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d", resp.StatusCode)
	}
	if conv.got.Page != 4 || conv.got.DPI != 1.5 {
		t.Errorf("page/dpi = %d / %v", conv.got.Page, conv.got.DPI)
	}
	if resp.Header.Get("X-Total-Pages") != "12" {
		t.Errorf("X-Total-Pages = %q", resp.Header.Get("X-Total-Pages"))
	}
}

func TestPNGHandlerDefaults(t *testing.T) {
	conv := &fakeConverter{body: []byte("PNG"), mime: "image/png", pages: 1}
	h := server.New(server.Deps{Conv: conv, MaxUploadBytes: 1 << 20})
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest("POST", srv.URL+"/v1/convert/png", strings.NewReader("x"))
	req.Header.Set("Content-Type", "application/x-test")
	_, _ = http.DefaultClient.Do(req)
	if conv.got.Page != 0 || conv.got.DPI != 1.0 {
		t.Errorf("defaults: page=%d dpi=%v", conv.got.Page, conv.got.DPI)
	}
}

func TestPNGHandlerRejectsBadParams(t *testing.T) {
	conv := &fakeConverter{body: []byte("PNG"), mime: "image/png", pages: 1}
	h := server.New(server.Deps{Conv: conv, MaxUploadBytes: 1 << 20})
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest("POST", srv.URL+"/v1/convert/png?page=abc", strings.NewReader("x"))
	req.Header.Set("Content-Type", "application/x-test")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestThumbnailDefaultsToPage0LowDPI(t *testing.T) {
	conv := &fakeConverter{body: []byte("PNG"), mime: "image/png", pages: 5}
	h := server.New(server.Deps{Conv: conv, MaxUploadBytes: 1 << 20})
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest("POST", srv.URL+"/v1/thumbnail", strings.NewReader("x"))
	req.Header.Set("Content-Type", "application/x-test")
	_, _ = http.DefaultClient.Do(req)
	if conv.got.Page != 0 || conv.got.DPI != 0.5 {
		t.Errorf("thumbnail defaults: page=%d dpi=%v", conv.got.Page, conv.got.DPI)
	}
}

func TestConvertPNGAcceptsPDFInput(t *testing.T) {
	conv := &fakeConverter{body: []byte("\x89PNG\r\n\x1a\n"), mime: "image/png"}
	h := server.New(server.Deps{Conv: conv, MaxUploadBytes: 1 << 20})
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest("POST", srv.URL+"/v1/convert/png?page=0&dpi=1.0", strings.NewReader("%PDF-1.3"))
	req.Header.Set("Content-Type", "application/pdf")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if conv.got.Format != worker.FormatPNG {
		t.Errorf("dispatched job Format = %v, want FormatPNG", conv.got.Format)
	}
}
