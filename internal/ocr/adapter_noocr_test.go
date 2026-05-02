//go:build noocr

package ocr

import (
	"context"
	"errors"
	"testing"
)

func TestNoocrNewReturnsUnavailable(t *testing.T) {
	_, err := New(Config{TessdataPath: "/nope", Languages: SupportedLangs, DPI: 300})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("New under noocr returned %v, want ErrUnavailable", err)
	}
}

func TestNoocrEngineRecognizeFails(t *testing.T) {
	e := noocrEngine{}
	if _, err := e.Recognize(context.Background(), nil, "eng"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Recognize under noocr returned %v, want ErrUnavailable", err)
	}
	if err := e.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}
