package server

import (
	"context"
	_ "embed"
	"errors"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/julianshen/bi/internal/worker"
	"golang.org/x/sync/singleflight"
)

//go:embed health_fixture.bin
var healthFixture []byte

type readyzCache struct {
	mu      sync.Mutex
	last    time.Time
	lastErr error
	ttl     time.Duration
	flight  singleflight.Group
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

	// singleflight collapses concurrent cache-miss probes to one in-flight
	// LO conversion. Without it, a thundering herd of /readyz hits during
	// startup could fan out N probes that each consume a worker for up to
	// ConvertTimeout (default 120s).
	v, _, _ := s.readyzC.flight.Do("readyz", func() (any, error) {
		err := s.runReadyzProbe(r.Context())
		s.readyzC.mu.Lock()
		s.readyzC.last = time.Now()
		s.readyzC.lastErr = err
		s.readyzC.mu.Unlock()
		return err, nil
	})
	probeErr, _ := v.(error)
	s.respondReady(w, probeErr)
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
		return errors.New("readyz: converter not wired")
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
