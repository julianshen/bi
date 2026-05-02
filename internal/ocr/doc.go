// Package ocr exposes a minimal Engine interface for optical
// character recognition of rendered PDF pages, used by the markdown
// conversion path on scanned PDFs. The cgo-backed gosseract
// implementation is built unless -tags=noocr is set; the stub
// returned under noocr always reports the engine as unavailable.
//
// This package must remain importable from pure-Go callers
// (handlers, config); cgo lives only in gosseract_adapter.go.
package ocr
