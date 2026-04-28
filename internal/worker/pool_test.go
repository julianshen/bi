package worker

import (
	"errors"
	"testing"
	"time"
)

func TestNewWithFakeOfficeStartsAndCloses(t *testing.T) {
	office := &fakeOffice{}
	p, err := newWithOffice(Config{Workers: 2, QueueDepth: 4, ConvertTimeout: time.Second}, office)
	if err != nil {
		t.Fatalf("newWithOffice: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if office.closeCalls != 1 {
		t.Fatalf("office.Close called %d times, want 1", office.closeCalls)
	}
}

func TestPoolCloseIsIdempotent(t *testing.T) {
	office := &fakeOffice{}
	p, _ := newWithOffice(Config{Workers: 1, QueueDepth: 1, ConvertTimeout: time.Second}, office)
	if err := p.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if office.closeCalls != 1 {
		t.Fatalf("office.Close called %d times, want 1", office.closeCalls)
	}
}

func TestPoolValidatesConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{"workers zero", Config{Workers: 0, QueueDepth: 1, ConvertTimeout: time.Second}},
		{"queue zero", Config{Workers: 1, QueueDepth: 0, ConvertTimeout: time.Second}},
		{"timeout zero", Config{Workers: 1, QueueDepth: 1, ConvertTimeout: 0}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := newWithOffice(c.cfg, &fakeOffice{})
			if err == nil {
				t.Fatal("want error, got nil")
			}
		})
	}
}

var errBoom = errors.New("boom")

func TestPoolCloseSurfacesOfficeErr(t *testing.T) {
	office := &fakeOffice{closeErr: errBoom}
	p, _ := newWithOffice(Config{Workers: 1, QueueDepth: 1, ConvertTimeout: time.Second}, office)
	if err := p.Close(); !errors.Is(err, errBoom) {
		t.Fatalf("Close err = %v, want errBoom", err)
	}
}
