// Package config loads and validates runtime configuration for the bi service.
//
// Configuration sources, in precedence order:
//  1. Command-line flags on the bi binary
//  2. Environment variables (LOK_PATH, BI_LISTEN_ADDR, BI_WORKERS, ...)
//  3. Platform-default candidates for the LibreOffice install path
//
// This package must remain pure Go — no cgo, no LOK imports — so it can be
// unit-tested without LibreOffice present.
package config
