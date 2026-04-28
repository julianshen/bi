package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthcheckExitCode(t *testing.T) {
	cases := []struct {
		status int
		want   int
	}{
		{200, 0},
		{503, 1},
		{500, 1},
	}
	for _, c := range cases {
		t.Run(http.StatusText(c.status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(c.status)
			}))
			t.Cleanup(srv.Close)
			got := healthcheckExit(srv.URL)
			if got != c.want {
				t.Errorf("exit = %d, want %d", got, c.want)
			}
		})
	}
}
