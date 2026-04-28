package worker

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPoolRunHonoursDeadline(t *testing.T) {
	doc := &fakeDocument{parts: 1}
	doc.saveAsHook = func(_, _, _ string) error {
		time.Sleep(200 * time.Millisecond)
		return nil
	}
	office := &fakeOffice{loadDoc: doc}
	p, _ := newWithOffice(Config{Workers: 1, QueueDepth: 1, ConvertTimeout: 50 * time.Millisecond}, office)
	t.Cleanup(func() { _ = p.Close() })

	in := tmpFile(t, "doc.docx", []byte("x"))
	_, err := p.Run(context.Background(), Job{InPath: in, Format: FormatPDF})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
}

func TestPoolRunHonoursCallerCancel(t *testing.T) {
	doc := &fakeDocument{parts: 1}
	doc.saveAsHook = func(_, _, _ string) error {
		time.Sleep(200 * time.Millisecond)
		return nil
	}
	office := &fakeOffice{loadDoc: doc}
	p, _ := newWithOffice(Config{Workers: 1, QueueDepth: 1, ConvertTimeout: time.Second}, office)
	t.Cleanup(func() { _ = p.Close() })

	in := tmpFile(t, "doc.docx", []byte("x"))
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	_, err := p.Run(ctx, Job{InPath: in, Format: FormatPDF})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
