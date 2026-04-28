package config_test

import "os"

func writeBytes(path string, b []byte) error {
	return os.WriteFile(path, b, 0o644)
}
