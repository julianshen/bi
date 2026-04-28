//go:build !nolok

package worker

import (
	"errors"
	"os"
)

func newRealOffice(_ string) (lokOffice, error) {
	return nil, errors.New("worker: real lok adapter not implemented yet")
}

func removeQuiet(path string) error { return os.Remove(path) }
