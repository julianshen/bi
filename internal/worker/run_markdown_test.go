package worker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeMD struct {
	got    []byte
	images MarkdownImageMode
	base   string
	marp   bool
	out    []byte
	err    error
}

func (f *fakeMD) Convert(html []byte, images MarkdownImageMode, base string, marp bool) ([]byte, error) {
	f.got = append(f.got, html...)
	f.images = images
	f.base = base
	f.marp = marp
	if f.err != nil {
		return nil, f.err
	}
	return f.out, nil
}

func TestPoolRunMarkdownHappyPath(t *testing.T) {
	doc := &fakeDocument{parts: 1}
	office := &fakeOffice{loadDoc: doc}
	p, _ := newWithOffice(Config{Workers: 1, QueueDepth: 1, ConvertTimeout: time.Second}, office)
	t.Cleanup(func() { _ = p.Close() })

	md := &fakeMD{out: []byte("# hello\n")}
	p.setMarkdown(md)

	var capturedHTMLPath string
	doc.saveAsHook = func(path, filter, _ string) error {
		if filter != "html" {
			t.Errorf("filter = %q, want html", filter)
		}
		capturedHTMLPath = path
		return os.WriteFile(path, []byte("<p>hello</p>"), 0o600)
	}

	in := tmpFile(t, "doc.docx", []byte("x"))
	res, err := p.Run(context.Background(), Job{InPath: in, Format: FormatMarkdown, MarkdownImages: MarkdownImagesEmbed})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(res.OutPath) })

	if res.MIME != "text/markdown" {
		t.Errorf("MIME = %q, want text/markdown", res.MIME)
	}
	if !strings.Contains(string(md.got), "<p>hello</p>") {
		t.Errorf("md.got = %q, want HTML body", md.got)
	}
	if md.images != MarkdownImagesEmbed {
		t.Errorf("md.images = %v, want Embed", md.images)
	}
	if want := filepath.Dir(capturedHTMLPath); md.base != want {
		t.Errorf("md.base = %q, want %q (filepath.Dir of html temp)", md.base, want)
	}
	if md.marp {
		t.Errorf("md.marp = true, want false (default)")
	}
	got, _ := os.ReadFile(res.OutPath)
	if string(got) != "# hello\n" {
		t.Errorf("file = %q, want '# hello\\n'", got)
	}
}

func TestPoolRunMarkdownPropagatesMarpFlag(t *testing.T) {
	doc := &fakeDocument{parts: 1}
	doc.saveAsHook = func(path, _, _ string) error {
		return os.WriteFile(path, []byte("<p>hi</p>"), 0o600)
	}
	office := &fakeOffice{loadDoc: doc}
	p, _ := newWithOffice(Config{Workers: 1, QueueDepth: 1, ConvertTimeout: time.Second}, office)
	t.Cleanup(func() { _ = p.Close() })

	md := &fakeMD{out: []byte("ok")}
	p.setMarkdown(md)

	in := tmpFile(t, "deck.pptx", []byte("x"))
	_, err := p.Run(context.Background(), Job{
		InPath:         in,
		Format:         FormatMarkdown,
		MarkdownMarp:   true,
		MarkdownImages: MarkdownImagesEmbed,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !md.marp {
		t.Error("md.marp = false, want true")
	}
}

func TestPoolRunMarkdownConvertError(t *testing.T) {
	doc := &fakeDocument{parts: 1}
	doc.saveAsHook = func(path, _, _ string) error { return os.WriteFile(path, []byte("<p>x</p>"), 0o600) }
	office := &fakeOffice{loadDoc: doc}
	p, _ := newWithOffice(Config{Workers: 1, QueueDepth: 1, ConvertTimeout: time.Second}, office)
	t.Cleanup(func() { _ = p.Close() })
	p.setMarkdown(&fakeMD{err: errors.New("boom")})

	in := tmpFile(t, "doc.docx", []byte("x"))
	_, err := p.Run(context.Background(), Job{InPath: in, Format: FormatMarkdown})
	if !errors.Is(err, ErrMarkdownConversion) {
		t.Fatalf("err = %v, want ErrMarkdownConversion", err)
	}
}
