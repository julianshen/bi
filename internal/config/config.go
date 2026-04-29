package config

import (
	"fmt"
	"runtime"
	"strconv"
	"time"
)

type Config struct {
	ListenAddr     string
	APIToken       string
	LOKPath        string
	Workers        int
	QueueDepth     int
	MaxUploadBytes int64
	ConvertTimeout time.Duration
	ReadyzCacheTTL time.Duration
}

// Load reads config from a string→string map (typically populated from
// os.Environ at the binary boundary). Defaults are applied for any unset key.
// LOKPath, if unset, is left empty for the caller to resolve via
// ResolveLOKPath.
func Load(env map[string]string) (Config, error) {
	c := Config{
		ListenAddr:     ":8080",
		MaxUploadBytes: 100 * 1024 * 1024,
		ConvertTimeout: 120 * time.Second,
		ReadyzCacheTTL: 5 * time.Second,
	}
	c.Workers = min(runtime.NumCPU(), 4)
	c.QueueDepth = c.Workers * 2

	if v, ok := env["BI_LISTEN_ADDR"]; ok {
		c.ListenAddr = v
	}
	if v, ok := env["BI_API_TOKEN"]; ok {
		c.APIToken = v
	}
	if v, ok := env["LOK_PATH"]; ok {
		c.LOKPath = v
	}
	if v, ok := env["BI_WORKERS"]; ok {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return c, fmt.Errorf("BI_WORKERS=%q: %w", v, err)
		}
		c.Workers = n
		c.QueueDepth = n * 2
	}
	if v, ok := env["BI_QUEUE_DEPTH"]; ok {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return c, fmt.Errorf("BI_QUEUE_DEPTH=%q: %w", v, err)
		}
		c.QueueDepth = n
	}
	if v, ok := env["BI_MAX_UPLOAD_BYTES"]; ok {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			return c, fmt.Errorf("BI_MAX_UPLOAD_BYTES=%q invalid", v)
		}
		c.MaxUploadBytes = n
	}
	if v, ok := env["BI_CONVERT_TIMEOUT"]; ok {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return c, fmt.Errorf("BI_CONVERT_TIMEOUT=%q: %w", v, err)
		}
		c.ConvertTimeout = d
	}
	if v, ok := env["BI_READYZ_CACHE_TTL"]; ok {
		d, err := time.ParseDuration(v)
		if err != nil || d < 0 {
			return c, fmt.Errorf("BI_READYZ_CACHE_TTL=%q: %w", v, err)
		}
		c.ReadyzCacheTTL = d
	}
	return c, nil
}
