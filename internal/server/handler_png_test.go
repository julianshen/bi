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

func TestPNGHandlerAcceptsSelectedPagesAndLayout(t *testing.T) {
	conv := &fakeConverter{body: []byte("PNG"), mime: "image/png", pages: 12}
	h := server.New(server.Deps{Conv: conv, MaxUploadBytes: 1 << 20})
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest("POST", srv.URL+"/v1/convert/png?pages=0,2,4&layout=2x2&dpi=1.5", strings.NewReader("x"))
	req.Header.Set("Content-Type", "application/x-test")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got, want := conv.got.Pages, []int{0, 2, 4}; !intSlicesEqual(got, want) {
		t.Fatalf("pages = %v, want %v", got, want)
	}
	if conv.got.GridCols != 2 || conv.got.GridRows != 2 {
		t.Fatalf("layout = %dx%d, want 2x2", conv.got.GridCols, conv.got.GridRows)
	}
	if conv.got.Page != 0 || conv.got.DPI != 1.5 {
		t.Fatalf("page/dpi = %d / %v, want 0 / 1.5", conv.got.Page, conv.got.DPI)
	}
}

func TestPNGHandlerDefaultsSelectedPagesToSingleRow(t *testing.T) {
	conv := &fakeConverter{body: []byte("PNG"), mime: "image/png", pages: 12}
	h := server.New(server.Deps{Conv: conv, MaxUploadBytes: 1 << 20})
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest("POST", srv.URL+"/v1/convert/png?pages=1,3,5", strings.NewReader("x"))
	req.Header.Set("Content-Type", "application/x-test")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got, want := conv.got.Pages, []int{1, 3, 5}; !intSlicesEqual(got, want) {
		t.Fatalf("pages = %v, want %v", got, want)
	}
	if conv.got.GridCols != 3 || conv.got.GridRows != 1 {
		t.Fatalf("layout = %dx%d, want 3x1", conv.got.GridCols, conv.got.GridRows)
	}
}

func TestPNGHandlerRejectsPagesAndPageTogether(t *testing.T) {
	conv := &fakeConverter{body: []byte("PNG"), mime: "image/png", pages: 12}
	h := server.New(server.Deps{Conv: conv, MaxUploadBytes: 1 << 20})
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest("POST", srv.URL+"/v1/convert/png?page=0&pages=0,1", strings.NewReader("x"))
	req.Header.Set("Content-Type", "application/x-test")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestPNGHandlerRejectsBadPagesAndLayout(t *testing.T) {
	cases := []string{
		"/v1/convert/png?pages=1,,2",
		"/v1/convert/png?pages=a",
		"/v1/convert/png?layout=2x2",
		"/v1/convert/png?pages=1,2,3&layout=1x2",
		"/v1/convert/png?pages=1,2&layout=0x2",
		"/v1/convert/png?pages=1,2&layout=2",
	}
	for _, path := range cases {
		t.Run(path, func(t *testing.T) {
			conv := &fakeConverter{body: []byte("PNG"), mime: "image/png", pages: 12}
			h := server.New(server.Deps{Conv: conv, MaxUploadBytes: 1 << 20})
			srv := httptest.NewServer(h.Routes())
			t.Cleanup(srv.Close)

			req, _ := http.NewRequest("POST", srv.URL+path, strings.NewReader("x"))
			req.Header.Set("Content-Type", "application/x-test")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", resp.StatusCode)
			}
			if conv.calls != 0 {
				t.Fatalf("converter calls = %d, want 0", conv.calls)
			}
		})
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

func intSlicesEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
