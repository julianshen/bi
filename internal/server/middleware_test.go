package server_test

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/julianshen/bi/internal/server"
)

// TestAccessLogDoesNotForgePasswordHeader pins that the access-log
// middleware doesn't set X-Bi-Password to "[REDACTED]" when no header
// was sent. The earlier unconditional set caused the converter to read
// "[REDACTED]" as the document password and hand it to LO, producing
// an "Unspecified Application Error" — issue #3 root cause.
func TestAccessLogDoesNotForgePasswordHeader(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	var seen string
	h := server.AccessLog(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("X-Bi-Password")
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("POST", "/v1/convert/pdf", strings.NewReader("x"))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if seen != "" {
		t.Errorf("downstream X-Bi-Password = %q, want empty (no header was sent)", seen)
	}
}

func TestRequestIDMiddlewareSetsHeader(t *testing.T) {
	called := false
	h := server.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if id := server.RequestIDFrom(r.Context()); id == "" {
			t.Error("no request id in context")
		}
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if !called {
		t.Fatal("downstream not called")
	}
	if rr.Header().Get("X-Bi-Request-Id") == "" {
		t.Error("X-Bi-Request-Id not set")
	}
}

func TestRequestIDPreservedFromInbound(t *testing.T) {
	h := server.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := server.RequestIDFrom(r.Context()); id != "abc" {
			t.Errorf("id = %q", id)
		}
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Bi-Request-Id", "abc")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Header().Get("X-Bi-Request-Id") != "abc" {
		t.Error("Reflected ID lost")
	}
}

func TestMaxBytesTrips413(t *testing.T) {
	h := server.MaxBytes(10)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), 413)
			return
		}
	}))
	req := httptest.NewRequest("POST", "/", bytes.NewReader([]byte(strings.Repeat("x", 100))))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 413 {
		t.Errorf("status = %d, want 413", rr.Code)
	}
}

func TestRecoverConvertsPanicTo500(t *testing.T) {
	h := server.Recover(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 500 {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

func TestAuthMiddlewareDisabledWhenTokenEmpty(t *testing.T) {
	h := server.Auth("")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/", nil))
	if rr.Code != 200 {
		t.Errorf("status = %d, want 200 (auth disabled)", rr.Code)
	}
}

func TestAuthMiddlewareRequiresHeader(t *testing.T) {
	h := server.Auth("secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/v1/convert/pdf", nil))
	if rr.Code != 401 {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestAuthMiddlewareRejectsWrongToken(t *testing.T) {
	h := server.Auth("secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v1/convert/pdf", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	h.ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestAuthMiddlewareAcceptsCorrectToken(t *testing.T) {
	h := server.Auth("secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer secret")
	h.ServeHTTP(rr, req)
	if rr.Code != 204 {
		t.Errorf("status = %d, want 204", rr.Code)
	}
}

func TestAccessLogRedactsPasswordHeader(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	h := server.AccessLog(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("POST", "/v1/convert/pdf", strings.NewReader("body"))
	req.Header.Set("X-Bi-Password", "supersecret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if strings.Contains(buf.String(), "supersecret") {
		t.Fatalf("log leaked password: %s", buf.String())
	}
	if !strings.Contains(buf.String(), `"path":"/v1/convert/pdf"`) {
		t.Fatalf("log missing path field: %s", buf.String())
	}
}
