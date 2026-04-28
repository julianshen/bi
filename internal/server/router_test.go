package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/julianshen/bi/internal/server"
)

func TestHealthzReturns200(t *testing.T) {
	h := server.New(server.Deps{})
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}
