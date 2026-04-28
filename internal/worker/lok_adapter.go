//go:build !nolok

package worker

import "errors"

// newRealOffice will be implemented in Task 32. Stubbed here so the package
// compiles for unit tests that wire fakes directly via newWithOffice.
func newRealOffice(_ string) (lokOffice, error) {
	return nil, errors.New("worker: real lok adapter not implemented yet")
}

// removeQuiet drops a file, ignoring errors. Defined here to avoid extra os
// imports in pool.go. Real impl lands with the lok adapter in Task 32.
func removeQuiet(path string) error { return nil }
