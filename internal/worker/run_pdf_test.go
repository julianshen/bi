package worker

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPoolRunPDFHappyPath(t *testing.T) {
	office := &fakeOffice{loadDoc: &fakeDocument{parts: 7}}
	p, _ := newWithOffice(Config{Workers: 1, QueueDepth: 1, ConvertTimeout: time.Second}, office)
	t.Cleanup(func() { _ = p.Close() })

	in := tmpFile(t, "doc.docx", []byte("dummy"))
	res, err := p.Run(context.Background(), Job{InPath: in, Format: FormatPDF})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(res.OutPath) })

	if res.MIME != "application/pdf" {
		t.Errorf("MIME = %q, want application/pdf", res.MIME)
	}
	if res.TotalPages != 7 {
		t.Errorf("TotalPages = %d, want 7", res.TotalPages)
	}
	if !strings.HasSuffix(res.OutPath, ".pdf") {
		t.Errorf("OutPath = %q, want .pdf suffix", res.OutPath)
	}
	if len(office.loadDoc.saveAsCalls) != 1 || office.loadDoc.saveAsCalls[0].Filter != "pdf" {
		t.Errorf("saveAsCalls = %+v, want one call with filter=pdf", office.loadDoc.saveAsCalls)
	}
	if office.loadDoc.closeCalls != 1 {
		t.Errorf("doc.Close called %d times, want 1", office.loadDoc.closeCalls)
	}
}

func TestPoolRunPDFLoadError(t *testing.T) {
	office := &fakeOffice{loadErr: errors.New("password required")}
	p, _ := newWithOffice(Config{Workers: 1, QueueDepth: 1, ConvertTimeout: time.Second}, office)
	t.Cleanup(func() { _ = p.Close() })

	in := tmpFile(t, "doc.docx", []byte("dummy"))
	_, err := p.Run(context.Background(), Job{InPath: in, Format: FormatPDF})
	if !errors.Is(err, ErrPasswordRequired) {
		t.Fatalf("err = %v, want ErrPasswordRequired", err)
	}
}

func TestPoolRunPDFSaveError(t *testing.T) {
	office := &fakeOffice{loadDoc: &fakeDocument{parts: 1, saveAsErr: errors.New("filter rejected")}}
	p, _ := newWithOffice(Config{Workers: 1, QueueDepth: 1, ConvertTimeout: time.Second}, office)
	t.Cleanup(func() { _ = p.Close() })

	in := tmpFile(t, "doc.docx", []byte("dummy"))
	_, err := p.Run(context.Background(), Job{InPath: in, Format: FormatPDF})
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("err = %v, want ErrUnsupportedFormat", err)
	}
}

func tmpFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := dir + "/" + name
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
