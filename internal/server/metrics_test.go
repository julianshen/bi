package server_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/julianshen/bi/internal/server"
)

func TestMetricsEndpointReturnsExposition(t *testing.T) {
	h := server.New(server.Deps{Conv: &fakeConverter{}})
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	// Trigger a healthz hit so we have something in counters.
	_, _ = http.Get(srv.URL + "/healthz")

	resp, _ := http.Get(srv.URL + "/metrics")
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "bi_requests_total") {
		t.Errorf("missing bi_requests_total in:\n%s", body)
	}
}

func TestOTelMiddlewareDoesNotBreakRouting(t *testing.T) {
	h := server.New(server.Deps{Conv: &fakeConverter{}})
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)
	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d", resp.StatusCode)
	}
}
