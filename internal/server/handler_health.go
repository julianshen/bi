package server

import (
	"context"
	_ "embed"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/julianshen/bi/internal/worker"
)

//go:embed health_fixture.bin
var healthFixture []byte

type readyzCache struct {
	mu      sync.Mutex
	last    time.Time
	lastErr error
	ttl     time.Duration
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	s.readyzC.mu.Lock()
	if !s.readyzC.last.IsZero() && time.Since(s.readyzC.last) < s.readyzC.ttl {
		err := s.readyzC.lastErr
		s.readyzC.mu.Unlock()
		s.respondReady(w, err)
		return
	}
	s.readyzC.mu.Unlock()

	// Run the probe outside the lock so concurrent /readyz requests don't
	// pile up behind a single ConvertTimeout-long conversion.
	err := s.runReadyzProbe(r.Context())

	s.readyzC.mu.Lock()
	s.readyzC.last = time.Now()
	s.readyzC.lastErr = err
	s.readyzC.mu.Unlock()
	s.respondReady(w, err)
}

func (s *Server) runReadyzProbe(ctx context.Context) error {
	tmp, err := os.CreateTemp("", "bi-ready-*.docx")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(healthFixture); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()
	if s.deps.Conv == nil {
		return nil // tests that don't supply a converter pass readyz
	}
	res, err := s.deps.Conv.Run(ctx, worker.Job{
		InPath: tmp.Name(),
		Format: worker.FormatPDF,
	})
	if err == nil {
		os.Remove(res.OutPath)
	}
	return err
}

func (s *Server) respondReady(w http.ResponseWriter, err error) {
	if err == nil {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
		return
	}
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte("not ready: " + err.Error()))
}
