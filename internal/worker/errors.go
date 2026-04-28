package worker

import (
	"errors"
	"strings"

	"github.com/julianshen/golibreofficekit/lok"
)

var (
	ErrQueueFull          = errors.New("worker: queue full")
	ErrPasswordRequired   = errors.New("worker: password required")
	ErrWrongPassword      = errors.New("worker: wrong password")
	ErrUnsupportedFormat  = errors.New("worker: unsupported document")
	ErrLOKUnsupported     = errors.New("worker: LOK build lacks required slot")
	ErrMarkdownConversion = errors.New("worker: markdown pipeline failed")
)

// ErrLokUnsupportedRaw is the upstream sentinel re-exported for tests so they
// don't have to import lok. Keep this in sync with lok.ErrUnsupported — the
// classify() function checks errors.Is against the upstream value.
var ErrLokUnsupportedRaw = lok.ErrUnsupported

// Classify normalises an error from the lok call surface into one of the
// worker sentinels. Unknown errors are returned unchanged so callers can
// log them verbatim and metrics counters can label them "internal".
//
// Order matters: typed sentinels checked first, then string-sniffing on
// LOK's free-form error text. The string match is the only signal LOK
// gives for password and parse failures; isolated here so future upstream
// typed errors land as a one-file diff.
func Classify(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, lok.ErrUnsupported) {
		return ErrLOKUnsupported
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "wrong password"):
		return ErrWrongPassword
	case strings.Contains(msg, "password"):
		return ErrPasswordRequired
	}
	// LOK failed to load/save and it wasn't password-related → treat as
	// "we cannot handle this document" rather than internal error.
	if _, ok := err.(interface{ LOK() bool }); ok {
		return ErrUnsupportedFormat
	}
	// Heuristic: any non-stdlib error that mentions "filter" comes from LO.
	if strings.Contains(msg, "filter") || strings.Contains(msg, "load failed") {
		return ErrUnsupportedFormat
	}
	return err
}

var errNotImplemented = errors.New("worker: not implemented")

var (
	ErrPageOutOfRange = errors.New("worker: page out of range")
	ErrInvalidDPI     = errors.New("worker: invalid dpi")
)
