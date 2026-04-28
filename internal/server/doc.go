// Package server hosts the HTTP API for the bi service.
//
// Handlers accept multipart uploads, persist them to a temp file, submit a
// conversion job to the worker package, and stream the result back to the
// client. This package must not import golibreofficekit; all LOK access is
// mediated by an interface satisfied by internal/worker.
package server
