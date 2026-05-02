package worker

import (
	"context"

	"github.com/julianshen/bi/internal/ocr"
)

// fakeOCR is a recording stub used by the OCR-aware markdown tests.
// scriptedTexts maps a "page key" to the text returned. When unset,
// Recognize returns "FAKE_OCR_TEXT".
type fakeOCR struct {
	calls       []fakeOCRCall
	textsByCall []string
	errsByCall  []error
}

type fakeOCRCall struct {
	Lang string
	Size int
}

func (f *fakeOCR) Recognize(ctx context.Context, image []byte, langs string) (string, error) {
	idx := len(f.calls)
	f.calls = append(f.calls, fakeOCRCall{Lang: langs, Size: len(image)})
	if idx < len(f.errsByCall) && f.errsByCall[idx] != nil {
		return "", f.errsByCall[idx]
	}
	if idx < len(f.textsByCall) {
		return f.textsByCall[idx], nil
	}
	return "FAKE_OCR_TEXT", nil
}

func (f *fakeOCR) Close() error { return nil }

// Compile-time assertion.
var _ ocr.Engine = (*fakeOCR)(nil)
