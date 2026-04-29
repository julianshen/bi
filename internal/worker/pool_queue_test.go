package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestPoolQueueFullReturnsErr(t *testing.T) {
	// 1 worker, queue depth 1. Steady state: worker holds one job inside
	// SaveAs (gate-blocked); one more job sits in the queue. Any further
	// Run hits the queue cap and returns ErrQueueFull. Use barriers
	// (saveAsEntered, queue-length poll) instead of a time.Sleep so the
	// invariant holds under any scheduler.
	doc := &fakeDocument{parts: 1}
	gate := make(chan struct{})
	saveAsEntered := make(chan struct{}, 1)
	doc.saveAsHook = func(_, _, _ string) error {
		select {
		case saveAsEntered <- struct{}{}:
		default:
		}
		<-gate
		return nil
	}
	office := &fakeOffice{loadDoc: doc}
	p, _ := newWithOffice(Config{Workers: 1, QueueDepth: 1, ConvertTimeout: 10 * time.Second}, office)
	t.Cleanup(func() {
		// Make sure workers can drain in case the body returned early.
		select {
		case <-gate:
		default:
			close(gate)
		}
		_ = p.Close()
	})

	in := tmpFile(t, "doc.docx", []byte("x"))

	// barrier: job 1 reaches saveAsHook (worker is busy)
	enqueued := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		close(enqueued)
		_, _ = p.Run(context.Background(), Job{InPath: in, Format: FormatPDF})
	}()
	<-enqueued
	select {
	case <-saveAsEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for first job to reach saveAsHook")
	}

	// barrier: job 2 enqueued. We can't observe enqueue completion directly;
	// poll queue length instead.
	wg.Add(1)
	job2Submitted := make(chan struct{})
	go func() {
		defer wg.Done()
		close(job2Submitted)
		_, _ = p.Run(context.Background(), Job{InPath: in, Format: FormatPDF})
	}()
	<-job2Submitted

	deadline := time.Now().Add(2 * time.Second)
	for {
		if len(p.queue) == cap(p.queue) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timeout waiting for queue to fill (job 2 did not enqueue)")
		}
		time.Sleep(time.Millisecond)
	}

	_, err := p.Run(context.Background(), Job{InPath: in, Format: FormatPDF})
	if !errors.Is(err, ErrQueueFull) {
		// Even on failure we must release the gate so the test cleanup
		// isn't stuck behind a worker forever.
		close(gate)
		wg.Wait()
		t.Fatalf("err = %v, want ErrQueueFull", err)
	}
	close(gate)
	wg.Wait()
}
