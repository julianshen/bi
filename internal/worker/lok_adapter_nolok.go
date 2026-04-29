//go:build nolok

// Stub variant of the real lok adapter. Selected with `-tags=nolok` for
// coverage runs that do not want the cgo pass-through file in the profile.
// Production builds use `lok_adapter.go` (no build tags).
package worker

import (
	"errors"
	"os"
)

func newRealOffice(_ string) (lokOffice, error) {
	return nil, errors.New("worker: built with -tags=nolok; lok adapter unavailable")
}

func removeQuiet(path string) error { return os.Remove(path) }
