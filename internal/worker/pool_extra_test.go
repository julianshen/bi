package worker

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeDocInitErr lets us script InitializeForRendering and RenderPagePNG
// failures without modifying the shared fakeDocument.
type fakeDocInitErr struct {
	*fakeDocument
	initErr error
}

func (f *fakeDocInitErr) InitializeForRendering(arg string) error {
	if f.initErr != nil {
		return f.initErr
	}
	return f.fakeDocument.InitializeForRendering(arg)
}

type officeReturning struct{ doc lokDocument }

func (o officeReturning) Load(string, string) (lokDocument, error) { return o.doc, nil }
func (o officeReturning) Close() error                              { return nil }

func TestNewSurfacesNewRealOfficeError(t *testing.T) {
	// Production New() calls newRealOffice which is a stub returning an error
	// until Task 32 wires the real adapter. The error must propagate.
	_, err := New(Config{LOKPath: "/nope", Workers: 1, QueueDepth: 1, ConvertTimeout: time.Second})
	if err == nil {
		t.Fatal("New: want error, got nil")
	}
}

func TestExecuteDefaultCaseForUnknownFormat(t *testing.T) {
	office := &fakeOffice{}
	p, _ := newWithOffice(Config{Workers: 1, QueueDepth: 1, ConvertTimeout: time.Second}, office)
	t.Cleanup(func() { _ = p.Close() })

	in := tmpFile(t, "doc.docx", []byte("x"))
	_, err := p.Run(context.Background(), Job{InPath: in, Format: Format(99)})
	if err == nil {
		t.Fatal("want error for unknown format")
	}
}

func TestRunMarkdownErrorsWhenMDNotWired(t *testing.T) {
	office := &fakeOffice{loadDoc: &fakeDocument{parts: 1}}
	p, _ := newWithOffice(Config{Workers: 1, QueueDepth: 1, ConvertTimeout: time.Second}, office)
	t.Cleanup(func() { _ = p.Close() })

	in := tmpFile(t, "doc.docx", []byte("x"))
	_, err := p.Run(context.Background(), Job{InPath: in, Format: FormatMarkdown})
	if err == nil {
		t.Fatal("want error when md is nil")
	}
}

func TestRunMarkdownLoadError(t *testing.T) {
	office := &fakeOffice{loadErr: errors.New("filter rejected file")}
	p, _ := newWithOffice(Config{Workers: 1, QueueDepth: 1, ConvertTimeout: time.Second}, office)
	t.Cleanup(func() { _ = p.Close() })
	p.setMarkdown(&fakeMD{out: []byte("ignored")})

	in := tmpFile(t, "doc.docx", []byte("x"))
	_, err := p.Run(context.Background(), Job{InPath: in, Format: FormatMarkdown})
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("err = %v, want ErrUnsupportedFormat", err)
	}
}

func TestRunMarkdownSaveAsHTMLError(t *testing.T) {
	doc := &fakeDocument{parts: 1, saveAsErr: errors.New("filter rejected file")}
	office := &fakeOffice{loadDoc: doc}
	p, _ := newWithOffice(Config{Workers: 1, QueueDepth: 1, ConvertTimeout: time.Second}, office)
	t.Cleanup(func() { _ = p.Close() })
	p.setMarkdown(&fakeMD{out: []byte("x")})

	in := tmpFile(t, "doc.docx", []byte("x"))
	_, err := p.Run(context.Background(), Job{InPath: in, Format: FormatMarkdown})
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("err = %v, want ErrUnsupportedFormat", err)
	}
}

func TestRunPNGLoadError(t *testing.T) {
	office := &fakeOffice{loadErr: errors.New("password required")}
	p, _ := newWithOffice(Config{Workers: 1, QueueDepth: 1, ConvertTimeout: time.Second}, office)
	t.Cleanup(func() { _ = p.Close() })

	in := tmpFile(t, "deck.pptx", []byte("x"))
	_, err := p.Run(context.Background(), Job{InPath: in, Format: FormatPNG, Page: 0, DPI: 1.0})
	if !errors.Is(err, ErrPasswordRequired) {
		t.Fatalf("err = %v, want ErrPasswordRequired", err)
	}
}

func TestRunPNGRenderError(t *testing.T) {
	doc := &fakeDocument{parts: 5, renderErr: errors.New("filter rejected")}
	office := &fakeOffice{loadDoc: doc}
	p, _ := newWithOffice(Config{Workers: 1, QueueDepth: 1, ConvertTimeout: time.Second}, office)
	t.Cleanup(func() { _ = p.Close() })

	in := tmpFile(t, "deck.pptx", []byte("x"))
	_, err := p.Run(context.Background(), Job{InPath: in, Format: FormatPNG, Page: 0, DPI: 1.0})
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("err = %v, want ErrUnsupportedFormat", err)
	}
}

func TestRunPNGCtxAlreadyDone(t *testing.T) {
	office := &fakeOffice{loadDoc: &fakeDocument{parts: 1}}
	p, _ := newWithOffice(Config{Workers: 1, QueueDepth: 1, ConvertTimeout: time.Second}, office)
	t.Cleanup(func() { _ = p.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	in := tmpFile(t, "deck.pptx", []byte("x"))
	_, err := p.Run(ctx, Job{InPath: in, Format: FormatPNG, Page: 0, DPI: 1.0})
	if err == nil {
		t.Fatal("want error for cancelled ctx")
	}
}

func TestRunMarkdownCtxAlreadyDone(t *testing.T) {
	office := &fakeOffice{loadDoc: &fakeDocument{parts: 1}}
	p, _ := newWithOffice(Config{Workers: 1, QueueDepth: 1, ConvertTimeout: time.Second}, office)
	t.Cleanup(func() { _ = p.Close() })
	p.setMarkdown(&fakeMD{out: []byte("x")})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	in := tmpFile(t, "doc.docx", []byte("x"))
	_, err := p.Run(ctx, Job{InPath: in, Format: FormatMarkdown})
	if err == nil {
		t.Fatal("want error for cancelled ctx")
	}
}

func TestRunPDFCtxAlreadyDone(t *testing.T) {
	office := &fakeOffice{loadDoc: &fakeDocument{parts: 1}}
	p, _ := newWithOffice(Config{Workers: 1, QueueDepth: 1, ConvertTimeout: time.Second}, office)
	t.Cleanup(func() { _ = p.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel; runPDF should observe ctx.Err early
	in := tmpFile(t, "doc.docx", []byte("x"))
	_, err := p.Run(ctx, Job{InPath: in, Format: FormatPDF})
	if err == nil {
		t.Fatal("want error for cancelled ctx")
	}
}
