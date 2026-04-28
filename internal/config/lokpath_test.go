package config_test

import (
	"errors"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/julianshen/bi/internal/config"
)

// TestResolveLOKPath pins the LOK install-path resolution contract.
//
// Precedence: explicit override > $LOK_PATH > platform defaults > error.
// The resolver only confirms a candidate is a directory; it does NOT verify
// the LO shared libraries are present (that is lok.New's job and produces a
// far better error message than we could).
func TestResolveLOKPath(t *testing.T) {
	tmp := t.TempDir()

	t.Run("explicit override wins", func(t *testing.T) {
		got, err := config.ResolveLOKPath(config.LOKPathSources{
			Override: tmp,
			Env:      "/should/be/ignored",
			Defaults: []string{"/also/ignored"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != tmp {
			t.Fatalf("got %q, want %q", got, tmp)
		}
	})

	t.Run("env used when no override", func(t *testing.T) {
		got, err := config.ResolveLOKPath(config.LOKPathSources{
			Env:      tmp,
			Defaults: []string{"/nope"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != tmp {
			t.Fatalf("got %q, want %q", got, tmp)
		}
	})

	t.Run("first existing default wins", func(t *testing.T) {
		got, err := config.ResolveLOKPath(config.LOKPathSources{
			Defaults: []string{
				filepath.Join(tmp, "missing"),
				tmp,
				filepath.Join(tmp, "also-missing"),
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != tmp {
			t.Fatalf("got %q, want %q", got, tmp)
		}
	})

	t.Run("no candidate found returns ErrLOKPathNotFound", func(t *testing.T) {
		_, err := config.ResolveLOKPath(config.LOKPathSources{
			Defaults: []string{filepath.Join(tmp, "a"), filepath.Join(tmp, "b")},
		})
		if !errors.Is(err, config.ErrLOKPathNotFound) {
			t.Fatalf("got %v, want ErrLOKPathNotFound", err)
		}
	})

	t.Run("non-directory candidate is rejected", func(t *testing.T) {
		// A regular file at tmp/file should not satisfy the contract.
		file := filepath.Join(tmp, "file")
		if err := writeEmpty(file); err != nil {
			t.Fatal(err)
		}
		_, err := config.ResolveLOKPath(config.LOKPathSources{
			Override: file,
		})
		if !errors.Is(err, config.ErrLOKPathNotFound) {
			t.Fatalf("got %v, want ErrLOKPathNotFound for non-directory", err)
		}
	})

	t.Run("PlatformDefaults returns non-empty list", func(t *testing.T) {
		got := config.PlatformDefaults()
		if len(got) == 0 {
			t.Fatalf("PlatformDefaults() returned empty slice on %s", runtime.GOOS)
		}
	})
}

func writeEmpty(path string) error {
	return writeBytes(path, nil)
}
