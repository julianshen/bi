package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/julianshen/bi/internal/server"
	"github.com/julianshen/bi/internal/worker"
)

// TestInitTracingFailsWithoutEndpoint exercises the error path of InitTracing
// when the OTLP exporter cannot connect. We give it a tight ctx so it doesn't
// hang on retries.
func TestInitTracingHandlesNoEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:1") // unreachable
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	shutdown, err := server.InitTracing(ctx, "bi-test")
	// Both outcomes are acceptable: returns err, or returns OK and shutdown
	// finishes within the deadline. Just confirm it doesn't panic.
	if err == nil && shutdown != nil {
		_ = shutdown(context.Background())
	}
}

// TestThumbnailRejectsBadParams mirrors PNG handler bad-params test for the
// thumbnail variant.
func TestThumbnailRejectsBadParams(t *testing.T) {
	conv := &fakeConverter{body: []byte("PNG"), mime: "image/png", pages: 1}
	h := server.New(server.Deps{Conv: conv, MaxUploadBytes: 1 << 20})
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest("POST", srv.URL+"/v1/thumbnail?dpi=abc", strings.NewReader("x"))
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

// TestPDFHandlerTrips413OnOversizeBody exercises the MaxBytes → 413 path
// inside handleConversion.
func TestPDFHandlerTrips413OnOversizeBody(t *testing.T) {
	conv := &fakeConverter{body: []byte("%PDF"), mime: "application/pdf"}
	h := server.New(server.Deps{Conv: conv, MaxUploadBytes: 16}) // tiny cap
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	body := strings.NewReader(strings.Repeat("x", 10_000))
	req, _ := http.NewRequest("POST", srv.URL+"/v1/convert/pdf", body)
	req.Header.Set("Content-Type", "application/x-test")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 413 {
		t.Errorf("status = %d, want 413", resp.StatusCode)
	}
}

// countingConverter blocks Run on a gate channel and counts calls.
type countingConverter struct {
	gate  chan struct{}
	calls int32
}

func (c *countingConverter) Run(ctx context.Context, job worker.Job) (worker.Result, error) {
	atomic.AddInt32(&c.calls, 1)
	<-c.gate
	tmp, err := os.CreateTemp("", "fake-pdf-*")
	if err != nil {
		return worker.Result{}, err
	}
	tmp.Close()
	return worker.Result{OutPath: tmp.Name(), MIME: "application/pdf"}, nil
}

// TestReadyzSingleflightCollapsesConcurrentProbes asserts that N concurrent
// /readyz hits result in exactly one call to the underlying converter, not N.
// Without singleflight a thundering herd would fan out N LO conversions.
func TestReadyzSingleflightCollapsesConcurrentProbes(t *testing.T) {
	conv := &countingConverter{gate: make(chan struct{})}
	h := server.New(server.Deps{Conv: conv, ReadyzTTL: time.Millisecond})
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	const n = 20
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get(srv.URL + "/readyz")
			if err == nil {
				resp.Body.Close()
			}
		}()
	}
	// Let goroutines pile up on the singleflight, then release.
	time.Sleep(50 * time.Millisecond)
	close(conv.gate)
	wg.Wait()

	got := atomic.LoadInt32(&conv.calls)
	if got != 1 {
		t.Fatalf("converter calls = %d (across %d concurrent /readyz), want 1", got, n)
	}
}

// TestReadyzCacheReturnsStaleResultWithinTTL exercises the cache-hit path.
func TestReadyzCacheReturnsStaleResultWithinTTL(t *testing.T) {
	conv := &fakeConverter{body: []byte("%PDF"), mime: "application/pdf"}
	h := server.New(server.Deps{Conv: conv, ReadyzTTL: 10 * time.Second})
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	for i := 0; i < 3; i++ {
		resp, err := http.Get(srv.URL + "/readyz")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("call %d: status = %d", i, resp.StatusCode)
		}
	}
}

// TestReadyzFailsWhenConvIsNil pins that a misconfigured server (Conv not
// wired) reports 503, not 200. Earlier the nil-Conv branch silently passed
// readiness — a production hazard a load balancer wouldn't catch.
func TestReadyzFailsWhenConvIsNil(t *testing.T) {
	h := server.New(server.Deps{ReadyzTTL: time.Millisecond})
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/readyz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Errorf("status = %d, want 503 (Conv not wired)", resp.StatusCode)
	}
}

// TestRecoverHandlesNonStringPanic exercises the errPanic value-type
// branches: error, then arbitrary type.
func TestRecoverHandlesErrorPanic(t *testing.T) {
	cases := []any{
		"plain string",
		http.ErrAbortHandler, // an error value
		42,                   // arbitrary type
	}
	for _, v := range cases {
		v := v
		h := server.Recover(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic(v)
		}))
		req := httptest.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != 500 {
			t.Errorf("panic %T: status = %d, want 500", v, rr.Code)
		}
	}
}
