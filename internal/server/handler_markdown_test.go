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
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestMarkdownAutoEnablesMarpForPPTX(t *testing.T) {
	conv := &fakeConverter{body: []byte("# H"), mime: "text/markdown"}
	h := server.New(server.Deps{Conv: conv, MaxUploadBytes: 1 << 20})
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest("POST", srv.URL+"/v1/convert/markdown", strings.NewReader("x"))
	req.Header.Set("Content-Type", "application/vnd.openxmlformats-officedocument.presentationml.presentation")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !conv.got.MarkdownMarp {
		t.Errorf("MarkdownMarp = false, want true (pptx input should auto-enable Marp)")
	}
}

func TestMarkdownAutoEnablesMarpForODP(t *testing.T) {
	conv := &fakeConverter{body: []byte("# H"), mime: "text/markdown"}
	h := server.New(server.Deps{Conv: conv, MaxUploadBytes: 1 << 20})
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest("POST", srv.URL+"/v1/convert/markdown", strings.NewReader("x"))
	req.Header.Set("Content-Type", "application/vnd.oasis.opendocument.presentation")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !conv.got.MarkdownMarp {
		t.Errorf("MarkdownMarp = false, want true (odp input should auto-enable Marp)")
	}
}

func TestMarkdownDisablesMarpForDOCX(t *testing.T) {
	conv := &fakeConverter{body: []byte("# H"), mime: "text/markdown"}
	h := server.New(server.Deps{Conv: conv, MaxUploadBytes: 1 << 20})
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest("POST", srv.URL+"/v1/convert/markdown", strings.NewReader("x"))
	req.Header.Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if conv.got.MarkdownMarp {
		t.Errorf("MarkdownMarp = true, want false (docx input should not enable Marp)")
	}
}
