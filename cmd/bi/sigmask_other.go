//go:build !linux

package main

// resetSignalMask is a no-op on non-Linux platforms. The inherited-mask
// pathology is Linux-specific (and only triggers in the deployed docker
// runtime); see sigmask_linux.go for the real implementation.
func resetSignalMask() {}
