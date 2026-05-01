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

func TestMarkdownAutoMarpByContentType(t *testing.T) {
	cases := []struct {
		name string
		ct   string
		want bool
	}{
		{"pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation", true},
		{"odp", "application/vnd.oasis.opendocument.presentation", true},
		{"ppt", "application/vnd.ms-powerpoint", true},
		{"docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			conv := &fakeConverter{body: []byte("# H"), mime: "text/markdown"}
			h := server.New(server.Deps{Conv: conv, MaxUploadBytes: 1 << 20})
			srv := httptest.NewServer(h.Routes())
			t.Cleanup(srv.Close)

			req, _ := http.NewRequest("POST", srv.URL+"/v1/convert/markdown", strings.NewReader("x"))
			req.Header.Set("Content-Type", c.ct)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if conv.got.MarkdownMarp != c.want {
				t.Errorf("MarkdownMarp = %v, want %v for %s", conv.got.MarkdownMarp, c.want, c.ct)
			}
		})
	}
}

func TestConvertMarkdownAcceptsPDFWithoutMarp(t *testing.T) {
	conv := &fakeConverter{body: []byte("# H"), mime: "text/markdown"}
	h := server.New(server.Deps{Conv: conv, MaxUploadBytes: 1 << 20})
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest("POST", srv.URL+"/v1/convert/markdown", strings.NewReader("%PDF-1.3"))
	req.Header.Set("Content-Type", "application/pdf")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if conv.got.Format != worker.FormatMarkdown {
		t.Errorf("Format = %v, want FormatMarkdown", conv.got.Format)
	}
	if conv.got.MarkdownMarp {
		t.Errorf("MarkdownMarp = true, want false (PDFs are not presentations)")
	}
}
