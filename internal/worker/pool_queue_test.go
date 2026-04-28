package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestPoolQueueFullReturnsErr(t *testing.T) {
	// 1 worker, queue depth 1: blocking the worker on the first job means
	// a third concurrent submit should hit the queue cap.
	doc := &fakeDocument{parts: 1}
	gate := make(chan struct{})
	doc.saveAsHook = func(_, _, _ string) error {
		<-gate
		return nil
	}
	office := &fakeOffice{loadDoc: doc}
	p, _ := newWithOffice(Config{Workers: 1, QueueDepth: 1, ConvertTimeout: 10 * time.Second}, office)
	t.Cleanup(func() {
		// gate may already be closed below; closing twice panics.
		// Use a flag to dodge double-close.
		_ = p.Close()
	})

	in := tmpFile(t, "doc.docx", []byte("x"))

	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			_, _ = p.Run(context.Background(), Job{InPath: in, Format: FormatPDF})
		}()
	}
	// Give the goroutines a moment to enqueue.
	time.Sleep(50 * time.Millisecond)

	_, err := p.Run(context.Background(), Job{InPath: in, Format: FormatPDF})
	if !errors.Is(err, ErrQueueFull) {
		t.Fatalf("err = %v, want ErrQueueFull", err)
	}
	close(gate)
	wg.Wait()
}
