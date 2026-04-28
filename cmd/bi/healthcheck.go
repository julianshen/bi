package main

import (
	"net/http"
	"os"
	"time"
)

func runHealthcheck(_ []string) {
	addr := os.Getenv("BI_LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	url := "http://localhost" + addr + "/readyz"
	os.Exit(healthcheckExit(url))
}

func healthcheckExit(url string) int {
	c := &http.Client{Timeout: 5 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		return 0
	}
	return 1
}
