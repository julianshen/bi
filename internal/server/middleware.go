package server

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"time"

	"github.com/oklog/ulid/v2"
)

type ctxKey int

const ctxRequestID ctxKey = iota

// RequestID either reflects an inbound X-Bi-Request-Id or generates a ULID.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Bi-Request-Id")
		if id == "" {
			id = ulid.Make().String()
		}
		w.Header().Set("X-Bi-Request-Id", id)
		ctx := context.WithValue(r.Context(), ctxRequestID, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(ctxRequestID).(string)
	return id
}

// MaxBytes wraps the request body in http.MaxBytesReader.
func MaxBytes(max int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, max)
			next.ServeHTTP(w, r)
		})
	}
}

// Recover converts panics to 500 with a problem+json body.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				rid := RequestIDFrom(r.Context())
				WriteProblem(w, r.URL.Path, rid, errPanic{rec})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type errPanic struct{ v any }

func (e errPanic) Error() string {
	if s, ok := e.v.(string); ok {
		return "panic: " + s
	}
	if er, ok := e.v.(error); ok {
		return "panic: " + er.Error()
	}
	return "panic"
}

// Auth gates access on a static bearer token. If token is empty, auth is
// disabled (no-op).
func Auth(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if token == "" {
			return next
		}
		want := []byte("Bearer " + token)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := []byte(r.Header.Get("Authorization"))
			if subtle.ConstantTimeCompare(got, want) != 1 {
				w.Header().Set("WWW-Authenticate", `Bearer realm="bi"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// AccessLog emits one structured JSON line per request.
func AccessLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: 200}

			// Replace with [REDACTED] only when a password was actually
			// sent — otherwise downstream handlers would read the
			// placeholder as a real password and pass it to the
			// converter, breaking unauthenticated requests (root cause
			// of issue #3 — LO would refuse to load with a wrong
			// password and crash the conversion).
			redacted := r.Header.Get("X-Bi-Password") != ""
			if redacted {
				r.Header.Set("X-Bi-Password", "[REDACTED]")
			}

			next.ServeHTTP(rec, r)

			logger.LogAttrs(r.Context(), slog.LevelInfo, "request",
				slog.String("request_id", RequestIDFrom(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Int64("latency_ms", time.Since(start).Milliseconds()),
				slog.Bool("password_present", redacted),
			)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
