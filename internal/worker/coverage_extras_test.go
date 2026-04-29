package worker

import (
	"os"
	"testing"
)

// TestRemoveQuietDeletesFile exercises the small filesystem helper.
func TestRemoveQuietDeletesFile(t *testing.T) {
	f, err := os.CreateTemp("", "bi-rq-*")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	path := f.Name()

	if err := removeQuiet(path); err != nil {
		t.Fatalf("removeQuiet: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected file removed; stat err = %v", err)
	}
}

// TestRemoveQuietOnMissingPath confirms behaviour when the file is gone.
// (The nolok-variant function returns os.Remove's error directly.)
func TestRemoveQuietOnMissingPath(t *testing.T) {
	// Just confirms the call returns without panicking; the error itself
	// is unchecked by callers (defer pattern).
	_ = removeQuiet("/tmp/bi-definitely-missing-" + t.Name())
}
