// Package worker owns the singleton *lok.Office and dispatches conversion
// jobs against it.
//
// This is the only package in the repository that imports
// github.com/julianshen/golibreofficekit/lok. Keeping cgo isolated here
// preserves the invariant that the HTTP and config layers stay
// LibreOffice-free and therefore unit-testable on hosts without LO
// installed.
//
// Concurrency model: one *lok.Office per process (LOK enforces this), with
// a bounded job queue providing backpressure. Although lok serialises every
// call through its own mutex, the queue exists so a flood of large renders
// cannot pin unlimited memory waiting their turn.
package worker
