// internal/server/fakes_test.go
package server_test

import (
	"context"
	"errors"
	"os"

	"github.com/julianshen/bi/internal/worker"
)

type fakeConverter struct {
	got   worker.Job
	body  []byte
	mime  string
	pages int
	err   error
	calls int
}

func (f *fakeConverter) Run(ctx context.Context, job worker.Job) (worker.Result, error) {
	f.calls++
	f.got = job
	if f.err != nil {
		return worker.Result{}, f.err
	}
	tmp, err := os.CreateTemp("", "fake-*")
	if err != nil {
		return worker.Result{}, err
	}
	if _, err := tmp.Write(f.body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return worker.Result{}, err
	}
	tmp.Close()
	return worker.Result{OutPath: tmp.Name(), MIME: f.mime, TotalPages: f.pages}, nil
}

var _ worker.Converter = (*fakeConverter)(nil)
var _ = errors.New
