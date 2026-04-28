package config_test

import (
	"testing"
	"time"

	"github.com/julianshen/bi/internal/config"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := config.Load(map[string]string{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ListenAddr != ":8080" {
		t.Errorf("ListenAddr = %q, want :8080", cfg.ListenAddr)
	}
	if cfg.MaxUploadBytes != 100*1024*1024 {
		t.Errorf("MaxUploadBytes = %d, want 100MiB", cfg.MaxUploadBytes)
	}
	if cfg.ConvertTimeout != 120*time.Second {
		t.Errorf("ConvertTimeout = %v, want 120s", cfg.ConvertTimeout)
	}
	if cfg.ReadyzCacheTTL != 5*time.Second {
		t.Errorf("ReadyzCacheTTL = %v, want 5s", cfg.ReadyzCacheTTL)
	}
	if cfg.Workers <= 0 {
		t.Errorf("Workers = %d, want > 0", cfg.Workers)
	}
	if cfg.QueueDepth != cfg.Workers*2 {
		t.Errorf("QueueDepth = %d, want %d (= 2 × workers)", cfg.QueueDepth, cfg.Workers*2)
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	env := map[string]string{
		"BI_LISTEN_ADDR":      "127.0.0.1:9000",
		"BI_API_TOKEN":        "secret",
		"BI_WORKERS":          "8",
		"BI_QUEUE_DEPTH":      "32",
		"BI_MAX_UPLOAD_BYTES": "1048576",
		"BI_CONVERT_TIMEOUT":  "30s",
		"BI_READYZ_CACHE_TTL": "10s",
		"LOK_PATH":            "/opt/libreoffice/program",
	}
	cfg, err := config.Load(env)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ListenAddr != "127.0.0.1:9000" {
		t.Errorf("ListenAddr = %q", cfg.ListenAddr)
	}
	if cfg.APIToken != "secret" {
		t.Errorf("APIToken = %q", cfg.APIToken)
	}
	if cfg.Workers != 8 || cfg.QueueDepth != 32 {
		t.Errorf("Workers/QueueDepth = %d/%d", cfg.Workers, cfg.QueueDepth)
	}
	if cfg.MaxUploadBytes != 1<<20 {
		t.Errorf("MaxUploadBytes = %d", cfg.MaxUploadBytes)
	}
	if cfg.ConvertTimeout != 30*time.Second {
		t.Errorf("ConvertTimeout = %v", cfg.ConvertTimeout)
	}
	if cfg.ReadyzCacheTTL != 10*time.Second {
		t.Errorf("ReadyzCacheTTL = %v", cfg.ReadyzCacheTTL)
	}
	if cfg.LOKPath != "/opt/libreoffice/program" {
		t.Errorf("LOKPath = %q", cfg.LOKPath)
	}
}

func TestLoadInvalidEnv(t *testing.T) {
	cases := map[string]map[string]string{
		"workers nan":      {"BI_WORKERS": "abc"},
		"timeout no unit":  {"BI_CONVERT_TIMEOUT": "30"},
		"size negative":    {"BI_MAX_UPLOAD_BYTES": "-1"},
	}
	for name, env := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := config.Load(env); err == nil {
				t.Fatal("want error, got nil")
			}
		})
	}
}
