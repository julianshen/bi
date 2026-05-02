//go:build noocr

package ocr

import "context"

// New always returns ErrUnavailable when built with -tags=noocr.
// Used by `cover-gate` and any host without Tesseract installed.
func New(cfg Config) (Engine, error) { return nil, ErrUnavailable }

type noocrEngine struct{}

func (noocrEngine) Recognize(ctx context.Context, image []byte, langs string) (string, error) {
	return "", ErrUnavailable
}
func (noocrEngine) Close() error { return nil }
