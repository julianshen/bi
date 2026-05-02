package config

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenAddr       string
	APIToken         string
	LOKPath          string
	Workers          int
	QueueDepth       int
	MaxUploadBytes   int64
	ConvertTimeout   time.Duration
	ReadyzCacheTTL   time.Duration
	OCREnabled       bool
	OCRTessdataPath  string
	OCRTextThreshold int
	OCRDPI           float64
}

// Load reads config from a string→string map (typically populated from
// os.Environ at the binary boundary). Defaults are applied for any unset key.
// LOKPath, if unset, is left empty for the caller to resolve via
// ResolveLOKPath.
func Load(env map[string]string) (Config, error) {
	c := Config{
		ListenAddr:       ":8080",
		MaxUploadBytes:   100 * 1024 * 1024,
		ConvertTimeout:   120 * time.Second,
		ReadyzCacheTTL:   5 * time.Second,
		OCREnabled:       true,
		OCRTextThreshold: 16,
		OCRDPI:           300,
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
	if v, ok := env["BI_OCR_ENABLED"]; ok {
		switch strings.ToLower(v) {
		case "true", "1", "yes":
			c.OCREnabled = true
		case "false", "0", "no":
			c.OCREnabled = false
		default:
			return c, fmt.Errorf("BI_OCR_ENABLED=%q invalid", v)
		}
	}
	if v, ok := env["BI_OCR_TESSDATA"]; ok {
		c.OCRTessdataPath = v
	} else if v, ok := env["TESSDATA_PREFIX"]; ok {
		c.OCRTessdataPath = v
	}
	if v, ok := env["BI_OCR_THRESHOLD"]; ok {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return c, fmt.Errorf("BI_OCR_THRESHOLD=%q invalid", v)
		}
		c.OCRTextThreshold = n
	}
	if v, ok := env["BI_OCR_DPI"]; ok {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f <= 0 {
			return c, fmt.Errorf("BI_OCR_DPI=%q invalid", v)
		}
		c.OCRDPI = f
	}
	return c, nil
}
