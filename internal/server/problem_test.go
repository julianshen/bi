package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/julianshen/bi/internal/server"
	"github.com/julianshen/bi/internal/worker"
)

func TestWriteProblemFromError(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantSlug string
		wantStat int
	}{
		{"queue full", worker.ErrQueueFull, "queue-full", 429},
		{"pool closed", worker.ErrPoolClosed, "shutting-down", 503},
		{"markdown pipeline", worker.ErrMarkdownConversion, "markdown-pipeline", 500},
		{"markdown pipeline wrapped", fmt.Errorf("%w: io fail", worker.ErrMarkdownConversion), "markdown-pipeline", 500},
		{"password required", worker.ErrPasswordRequired, "password-required", 422},
		{"wrong password", worker.ErrWrongPassword, "password-wrong", 422},
		{"unsupported document", worker.ErrUnsupportedFormat, "unsupported-document", 422},
		{"lok unsupported", worker.ErrLOKUnsupported, "lok-unsupported", 501},
		{"page out of range", worker.ErrPageOutOfRange, "bad-request", 400},
		{"invalid dpi", worker.ErrInvalidDPI, "bad-request", 400},
		{"deadline", context.DeadlineExceeded, "timeout", 504},
		{"unknown", errors.New("unexpected"), "internal", 500},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			server.WriteProblem(rr, "/v1/convert/pdf", "req-1", c.err)
			if rr.Code != c.wantStat {
				t.Errorf("status = %d, want %d", rr.Code, c.wantStat)
			}
			if got := rr.Header().Get("Content-Type"); got != "application/problem+json" {
				t.Errorf("Content-Type = %q", got)
			}
			var p server.Problem
			if err := json.Unmarshal(rr.Body.Bytes(), &p); err != nil {
				t.Fatal(err)
			}
			if p.Type != "https://bi/errors/"+c.wantSlug {
				t.Errorf("Type = %q, want suffix %q", p.Type, c.wantSlug)
			}
			if p.RequestID != "req-1" {
				t.Errorf("RequestID = %q", p.RequestID)
			}
		})
	}
}

func TestWriteProblemQueueFullSetsRetryAfter(t *testing.T) {
	rr := httptest.NewRecorder()
	server.WriteProblem(rr, "/x", "r", worker.ErrQueueFull)
	if got := rr.Header().Get("Retry-After"); got != "1" {
		t.Errorf("Retry-After = %q", got)
	}
}
