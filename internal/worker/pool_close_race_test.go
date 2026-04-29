package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// TestPoolRunAfterCloseReturnsErrPoolClosed pins the contract: Close
// transitions the pool to a state where Run returns ErrPoolClosed instead
// of panicking on send-to-closed-channel.
func TestPoolRunAfterCloseReturnsErrPoolClosed(t *testing.T) {
	office := &fakeOffice{loadDoc: &fakeDocument{parts: 1}}
	p, _ := newWithOffice(Config{Workers: 1, QueueDepth: 1, ConvertTimeout: time.Second}, office)
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}

	in := tmpFile(t, "doc.docx", []byte("x"))
	_, err := p.Run(context.Background(), Job{InPath: in, Format: FormatPDF})
	if !errors.Is(err, ErrPoolClosed) {
		t.Fatalf("err = %v, want ErrPoolClosed", err)
	}
}

// TestPoolConcurrentRunCloseDoesNotPanic exercises the race directly. Even
// a single panic in this loop will fail the test (the deferred recover
// promotes a panic to a t.Errorf).
func TestPoolConcurrentRunCloseDoesNotPanic(t *testing.T) {
	for trial := 0; trial < 50; trial++ {
		office := &fakeOffice{loadDoc: &fakeDocument{parts: 1}}
		p, _ := newWithOffice(Config{Workers: 1, QueueDepth: 1, ConvertTimeout: time.Second}, office)
		in := tmpFile(t, "doc.docx", []byte("x"))

		var wg sync.WaitGroup
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("Run panicked: %v", r)
					}
				}()
				_, _ = p.Run(context.Background(), Job{InPath: in, Format: FormatPDF})
			}()
		}
		// Close concurrently with the in-flight Runs. Wait for it explicitly
		// so a Close-goroutine doesn't leak across trial iterations.
		wg.Add(1)
		go func() { defer wg.Done(); _ = p.Close() }()
		wg.Wait()
	}
}
