package config

import (
	"errors"
	"os"
	"runtime"
)

// ErrLOKPathNotFound indicates no candidate LibreOffice install path was a
// usable directory. The caller should surface this with the candidates list
// so the operator can see what was tried.
var ErrLOKPathNotFound = errors.New("LibreOffice install path not found")

// LOKPathSources groups the inputs to ResolveLOKPath. Callers populate the
// fields they have; empty fields are skipped.
type LOKPathSources struct {
	Override string   // Explicit value (e.g. CLI flag); highest precedence.
	Env      string   // Value of $LOK_PATH; checked when Override is empty.
	Defaults []string // Platform-default candidates; checked last, in order.
}

// ResolveLOKPath returns the first source that resolves to an existing
// directory. It does NOT verify that LibreOffice's shared libraries are
// present inside — that check belongs to lok.New, which produces a much
// better error than this package could.
func ResolveLOKPath(s LOKPathSources) (string, error) {
	candidates := make([]string, 0, 2+len(s.Defaults))
	if s.Override != "" {
		candidates = append(candidates, s.Override)
	}
	if s.Env != "" {
		candidates = append(candidates, s.Env)
	}
	candidates = append(candidates, s.Defaults...)

	for _, c := range candidates {
		if isDir(c) {
			return c, nil
		}
	}
	return "", ErrLOKPathNotFound
}

// PlatformDefaults returns the canonical LibreOffice program/ directory
// candidates for the current GOOS, in probe order.
func PlatformDefaults() []string {
	return platformDefaultsFor(runtime.GOOS)
}

func platformDefaultsFor(goos string) []string {
	switch goos {
	case "linux":
		return []string{
			"/usr/lib/libreoffice/program",   // Debian/Ubuntu/Arch/openSUSE
			"/usr/lib64/libreoffice/program", // Fedora/RHEL
		}
	case "darwin":
		return []string{
			"/Applications/LibreOffice.app/Contents/Frameworks",
			"/opt/homebrew/Caskroom/libreoffice/latest/LibreOffice.app/Contents/Frameworks",
		}
	default:
		return []string{}
	}
}

func isDir(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
