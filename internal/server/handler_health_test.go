package server_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/julianshen/bi/internal/server"
)

func TestReadyzReturns200OnHealthyConverter(t *testing.T) {
	conv := &fakeConverter{body: []byte("%PDF"), mime: "application/pdf"}
	h := server.New(server.Deps{Conv: conv, ReadyzTTL: time.Millisecond})
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	resp, _ := http.Get(srv.URL + "/readyz")
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

func TestReadyzReturns503OnConverterErr(t *testing.T) {
	conv := &fakeConverter{err: errors.New("LO down")}
	h := server.New(server.Deps{Conv: conv, ReadyzTTL: time.Millisecond})
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	resp, _ := http.Get(srv.URL + "/readyz")
	defer resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Errorf("status = %d", resp.StatusCode)
	}
}
