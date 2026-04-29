package worker

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

type fakeMD struct {
	got    []byte
	images MarkdownImageMode
	base   string
	out    []byte
	err    error
}

func (f *fakeMD) Convert(html []byte, images MarkdownImageMode, base string) ([]byte, error) {
	f.got = append(f.got, html...)
	f.images = images
	f.base = base
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

	doc.saveAsHook = func(path, filter, _ string) error {
		if filter != "html" {
			t.Errorf("filter = %q, want html", filter)
		}
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
	got, _ := os.ReadFile(res.OutPath)
	if string(got) != "# hello\n" {
		t.Errorf("file = %q, want '# hello\\n'", got)
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
