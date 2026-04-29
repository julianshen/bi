package server_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/julianshen/bi/internal/server"
	"github.com/julianshen/bi/internal/worker"
)

func TestPDFHandlerHappyPath(t *testing.T) {
	conv := &fakeConverter{body: []byte("%PDF-1.4 fake"), mime: "application/pdf", pages: 3}
	h := server.New(server.Deps{Conv: conv, MaxUploadBytes: 1 << 20})
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	body := strings.NewReader("dummy docx bytes")
	req, _ := http.NewRequest("POST", srv.URL+"/v1/convert/pdf", body)
	req.Header.Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/pdf" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := resp.Header.Get("X-Total-Pages"); got != "3" {
		t.Errorf("X-Total-Pages = %q", got)
	}
	got, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(got, []byte("%PDF-1.4 fake")) {
		t.Errorf("body mismatch")
	}
	if conv.got.Format != worker.FormatPDF {
		t.Errorf("Format = %v, want PDF", conv.got.Format)
	}
	// LO refuses to load files without a recognisable extension. Pin that
	// the temp filename ends with ".docx" so a refactor that drops the
	// extensionFromContentType call can't silently regress.
	if !strings.HasSuffix(conv.got.InPath, ".docx") {
		t.Errorf("InPath = %q, want .docx suffix", conv.got.InPath)
	}
}

func TestPDFHandlerRejectsEmptyContentType(t *testing.T) {
	conv := &fakeConverter{body: []byte("x"), mime: "application/pdf"}
	h := server.New(server.Deps{Conv: conv, MaxUploadBytes: 1 << 20})
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest("POST", srv.URL+"/v1/convert/pdf", strings.NewReader("x"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 415 {
		t.Errorf("status = %d, want 415", resp.StatusCode)
	}
}
