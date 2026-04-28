package worker_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/julianshen/bi/internal/worker"
)

// fakeLOKErr mimics lok.LOKError.Error() shape — a free-form string from LO.
type fakeLOKErr struct{ msg string }

func (e fakeLOKErr) Error() string { return e.msg }
func (e fakeLOKErr) LOK() bool     { return true }

func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		in   error
		want error
	}{
		{"nil maps to nil", nil, nil},
		{"context deadline passes through", fmt.Errorf("ctx: %w", errExampleDeadline), errExampleDeadline},
		{"lok unsupported", worker.ErrLokUnsupportedRaw, worker.ErrLOKUnsupported},
		{"password keyword (lowercase)", fakeLOKErr{"password required to open"}, worker.ErrPasswordRequired},
		{"wrong password keyword", fakeLOKErr{"wrong password"}, worker.ErrWrongPassword},
		{"unparseable falls through", fakeLOKErr{"filter rejected file"}, worker.ErrUnsupportedFormat},
		{"unknown error preserves identity", errSomething, errSomething},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := worker.Classify(c.in)
			if !errors.Is(got, c.want) && got != c.want {
				t.Fatalf("Classify(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

var (
	errExampleDeadline = errors.New("test: deadline")
	errSomething       = errors.New("test: something else")
)
