package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestWithTimingRoundTrip(t *testing.T) {
	ctx := context.Background()
	timing := &Timing{}
	ctx = WithTiming(ctx, timing)
	timing.QueueWaitMs = 42
	timing.ConvertMs = 99

	got := TimingFrom(ctx)
	if got == nil {
		t.Fatal("TimingFrom returned nil")
	}
	if got.QueueWaitMs != 42 {
		t.Errorf("QueueWaitMs = %d, want 42", got.QueueWaitMs)
	}
	if got.ConvertMs != 99 {
		t.Errorf("ConvertMs = %d, want 99", got.ConvertMs)
	}
}

func TestTimingFromMissing(t *testing.T) {
	if got := TimingFrom(context.Background()); got != nil {
		t.Errorf("TimingFrom without WithTiming = %v, want nil", got)
	}
}

func TestErrorKindTable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"queue-full", ErrQueueFull, "queue-full"},
		{"pool-closed", ErrPoolClosed, "pool-closed"},
		{"password-required", ErrPasswordRequired, "password-required"},
		{"wrong-password", ErrWrongPassword, "wrong-password"},
		{"unsupported-document", ErrUnsupportedFormat, "unsupported-document"},
		{"lok-unsupported", ErrLOKUnsupported, "lok-unsupported"},
		{"page-out-of-range", ErrPageOutOfRange, "page-out-of-range"},
		{"invalid-dpi", ErrInvalidDPI, "invalid-dpi"},
		{"markdown-pipeline", ErrMarkdownConversion, "markdown-pipeline"},
		{"internal", context.DeadlineExceeded, "internal"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ErrorKind(c.err); got != c.want {
				t.Errorf("ErrorKind(%v) = %q, want %q", c.err, got, c.want)
			}
		})
	}
}

// fakeInstrumenter records calls for test assertions.
type fakeInstrumenter struct {
	mu            sync.Mutex
	queueWaits    []time.Duration
	convDurations []time.Duration
	depths        []int
	busyDeltas    []int
	errors        []string
}

func (f *fakeInstrumenter) QueueWait(format Format, d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queueWaits = append(f.queueWaits, d)
}

func (f *fakeInstrumenter) ConversionDuration(format Format, d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.convDurations = append(f.convDurations, d)
}

func (f *fakeInstrumenter) QueueDepth(d int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.depths = append(f.depths, d)
}

func (f *fakeInstrumenter) WorkerBusy(delta int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.busyDeltas = append(f.busyDeltas, delta)
}

func (f *fakeInstrumenter) LokError(kind string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errors = append(f.errors, kind)
}

func (f *fakeInstrumenter) getDepths() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]int, len(f.depths))
	copy(out, f.depths)
	return out
}

func (f *fakeInstrumenter) getBusyDeltas() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]int, len(f.busyDeltas))
	copy(out, f.busyDeltas)
	return out
}

func TestPoolInstrumentsQueueAndConversion(t *testing.T) {
	inst := &fakeInstrumenter{}
	doc := &fakeDocument{parts: 1}
	office := &fakeOffice{loadDoc: doc}
	p, _ := newWithOffice(Config{
		Workers:        1,
		QueueDepth:     1,
		ConvertTimeout: time.Second,
		Inst:           inst,
	}, office)
	t.Cleanup(func() { _ = p.Close() })

	in := tmpFile(t, "doc.docx", []byte("x"))
	_, err := p.Run(context.Background(), Job{InPath: in, Format: FormatPDF})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(inst.queueWaits) != 1 {
		t.Errorf("QueueWait calls = %d, want 1", len(inst.queueWaits))
	}
	if len(inst.convDurations) != 1 {
		t.Errorf("ConversionDuration calls = %d, want 1", len(inst.convDurations))
	}
	deltas := inst.getBusyDeltas()
	if len(deltas) != 2 || deltas[0] != 1 || deltas[1] != -1 {
		t.Errorf("WorkerBusy deltas = %v, want [1 -1]", deltas)
	}
}

func TestPoolInstrumentsLokError(t *testing.T) {
	inst := &fakeInstrumenter{}
	office := &fakeOffice{loadErr: ErrPasswordRequired}
	p, _ := newWithOffice(Config{
		Workers:        1,
		QueueDepth:     1,
		ConvertTimeout: time.Second,
		Inst:           inst,
	}, office)
	t.Cleanup(func() { _ = p.Close() })

	in := tmpFile(t, "doc.docx", []byte("x"))
	_, err := p.Run(context.Background(), Job{InPath: in, Format: FormatPDF})
	if err == nil {
		t.Fatal("expected error")
	}

	if len(inst.errors) != 1 || inst.errors[0] != "password-required" {
		t.Errorf("LokError calls = %v, want [password-required]", inst.errors)
	}
}

func TestPoolInstrumentsQueueDepth(t *testing.T) {
	inst := &fakeInstrumenter{}
	doc := &fakeDocument{parts: 1}
	// Block the worker so the queue fills.
	gate := make(chan struct{})
	doc.saveAsHook = func(_, _, _ string) error {
		<-gate
		return nil
	}
	office := &fakeOffice{loadDoc: doc}
	p, _ := newWithOffice(Config{
		Workers:        1,
		QueueDepth:     1,
		ConvertTimeout: 10 * time.Second,
		Inst:           inst,
	}, office)
	t.Cleanup(func() {
		close(gate)
		_ = p.Close()
	})

	in := tmpFile(t, "doc.docx", []byte("x"))
	// First job blocks the worker.
	go func() {
		_, _ = p.Run(context.Background(), Job{InPath: in, Format: FormatPDF})
	}()
	time.Sleep(50 * time.Millisecond)

	// Second job fills the queue (depth 1).
	go func() {
		_, _ = p.Run(context.Background(), Job{InPath: in, Format: FormatPDF})
	}()
	time.Sleep(50 * time.Millisecond)

	// Third job should hit queue full.
	_, err := p.Run(context.Background(), Job{InPath: in, Format: FormatPDF})
	if !errors.Is(err, ErrQueueFull) {
		t.Fatalf("err = %v, want ErrQueueFull", err)
	}

	// We should have at least one depth observation >0 and one ==0 on rejection.
	foundNonZero := false
	foundZero := false
	for _, d := range inst.getDepths() {
		if d > 0 {
			foundNonZero = true
		}
		if d == 0 {
			foundZero = true
		}
	}
	if !foundNonZero {
		t.Error("expected at least one QueueDepth observation > 0")
	}
	if !foundZero {
		t.Error("expected at least one QueueDepth observation == 0")
	}
}
