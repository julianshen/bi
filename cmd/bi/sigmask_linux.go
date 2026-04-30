//go:build linux

package main

import "golang.org/x/sys/unix"

// resetSignalMask clears the inherited signal mask. The empty mask matches
// what an interactive-shell child gets, so LO sees a "clean" signal
// environment and can install its own handlers. When `bi convert` is
// exec'd from `bi serve`, the child inherits the long-running server's
// blocked signals (Go runtime + net/http + otelhttp accumulate them);
// LO's init then crashes with "Unspecified Application Error" because
// handlers it tries to install land on blocked signals.
//
// Failures here are non-fatal — at worst, LO crashes the same way it
// would have anyway, and the parent surfaces that as a 500.
func resetSignalMask() {
	var empty unix.Sigset_t
	_ = unix.PthreadSigmask(unix.SIG_SETMASK, &empty, nil)
}
