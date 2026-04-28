package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/julianshen/bi/internal/server"
	"github.com/julianshen/bi/internal/worker"
)

func TestMarkdownDefaultsToEmbed(t *testing.T) {
	conv := &fakeConverter{body: []byte("# H"), mime: "text/markdown"}
	h := server.New(server.Deps{Conv: conv, MaxUploadBytes: 1 << 20})
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest("POST", srv.URL+"/v1/convert/markdown", strings.NewReader("x"))
	req.Header.Set("Content-Type", "application/x-test")
	_, _ = http.DefaultClient.Do(req)
	if conv.got.MarkdownImages != worker.MarkdownImagesEmbed {
		t.Errorf("MarkdownImages = %v, want Embed", conv.got.MarkdownImages)
	}
}

func TestMarkdownDropMode(t *testing.T) {
	conv := &fakeConverter{body: []byte("# H"), mime: "text/markdown"}
	h := server.New(server.Deps{Conv: conv, MaxUploadBytes: 1 << 20})
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest("POST", srv.URL+"/v1/convert/markdown?images=drop", strings.NewReader("x"))
	req.Header.Set("Content-Type", "application/x-test")
	_, _ = http.DefaultClient.Do(req)
	if conv.got.MarkdownImages != worker.MarkdownImagesDrop {
		t.Errorf("MarkdownImages = %v, want Drop", conv.got.MarkdownImages)
	}
}

func TestMarkdownRejectsUnknownImagesMode(t *testing.T) {
	conv := &fakeConverter{body: []byte("# H"), mime: "text/markdown"}
	h := server.New(server.Deps{Conv: conv, MaxUploadBytes: 1 << 20})
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest("POST", srv.URL+"/v1/convert/markdown?images=garbage", strings.NewReader("x"))
	req.Header.Set("Content-Type", "application/x-test")
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}
