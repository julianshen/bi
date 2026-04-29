package worker

import (
	"context"
	"errors"
	"fmt"
	"sync"

	mdconvpkg "github.com/julianshen/bi/internal/mdconv"
)

// Pool is the production Converter.
type Pool struct {
	cfg      Config
	office   lokOffice
	queue    chan jobEnvelope
	workers  sync.WaitGroup
	closeMu  sync.Mutex
	closed   bool
	closeErr error
	md       htmlToMarkdown
}

type jobEnvelope struct {
	ctx    context.Context
	job    Job
	result chan<- runOutcome
}

type runOutcome struct {
	res Result
	err error
}

// New initialises lok and returns a ready Pool. It returns an error if lok is
// already initialised in this process (LOK enforces one init per process).
func New(cfg Config) (*Pool, error) {
	office, err := newRealOffice(cfg.LOKPath)
	if err != nil {
		return nil, fmt.Errorf("worker: init lok: %w", err)
	}
	p, err := newWithOffice(cfg, office)
	if err != nil {
		_ = office.Close()
		return nil, err
	}
	p.md = mdAdapter{}
	return p, nil
}

func newWithOffice(cfg Config, office lokOffice) (*Pool, error) {
	if cfg.Workers <= 0 {
		return nil, errors.New("worker: Workers must be > 0")
	}
	if cfg.QueueDepth <= 0 {
		return nil, errors.New("worker: QueueDepth must be > 0")
	}
	if cfg.ConvertTimeout <= 0 {
		return nil, errors.New("worker: ConvertTimeout must be > 0")
	}
	p := &Pool{
		cfg:    cfg,
		office: office,
		queue:  make(chan jobEnvelope, cfg.QueueDepth),
	}
	for i := 0; i < cfg.Workers; i++ {
		p.workers.Add(1)
		go p.runWorker()
	}
	return p, nil
}

func (p *Pool) runWorker() {
	defer p.workers.Done()
	for env := range p.queue {
		res, err := p.execute(env.ctx, env.job)
		select {
		case env.result <- runOutcome{res, err}:
		case <-env.ctx.Done():
			if res.OutPath != "" {
				_ = removeQuiet(res.OutPath)
			}
		}
	}
}

// execute is the per-format dispatcher. Implementations live in run_*.go.
func (p *Pool) execute(ctx context.Context, job Job) (Result, error) {
	switch job.Format {
	case FormatPDF:
		return p.runPDF(ctx, job)
	case FormatPNG:
		return p.runPNG(ctx, job)
	case FormatMarkdown:
		return p.runMarkdown(ctx, job)
	default:
		return Result{}, fmt.Errorf("worker: unknown format %d", job.Format)
	}
}

// Close stops accepting jobs, waits for in-flight work, then closes the
// underlying lok.Office. Idempotent. The lock is held across the entire
// shutdown so a racing second caller observes the fully-populated
// closeErr instead of the zero value.
func (p *Pool) Close() error {
	p.closeMu.Lock()
	defer p.closeMu.Unlock()
	if p.closed {
		return p.closeErr
	}
	p.closed = true
	close(p.queue)
	p.workers.Wait()
	p.closeErr = p.office.Close()
	return p.closeErr
}

// setMarkdown is a test helper for injecting an htmlToMarkdown.
func (p *Pool) setMarkdown(md htmlToMarkdown) { p.md = md }

// mdAdapter satisfies the worker's htmlToMarkdown seam by delegating to
// the internal/mdconv package.
type mdAdapter struct{}

func (mdAdapter) Convert(html []byte, mode MarkdownImageMode, base string) ([]byte, error) {
	var m mdconvpkg.ImageMode
	switch mode {
	case MarkdownImagesDrop:
		m = mdconvpkg.ImagesDrop
	default:
		m = mdconvpkg.ImagesEmbed
	}
	return mdconvpkg.ConvertWithBase(html, mdconvpkg.Options{Images: m}, base)
}

// Run submits a job and waits for the outcome. It honours ctx for both queue
// wait and the in-flight conversion. ctx.Err() takes precedence over the
// outcome on cancellation/timeout.
func (p *Pool) Run(ctx context.Context, job Job) (Result, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, p.cfg.ConvertTimeout)
	defer cancel()

	out := make(chan runOutcome, 1)
	env := jobEnvelope{ctx: timeoutCtx, job: job, result: out}

	// Hold closeMu across the enqueue so Close cannot close(p.queue)
	// between the select's evaluation and the actual send. Without this,
	// a concurrent Run + Close can panic on send-to-closed-channel.
	p.closeMu.Lock()
	if p.closed {
		p.closeMu.Unlock()
		return Result{}, ErrPoolClosed
	}
	select {
	case p.queue <- env:
		p.closeMu.Unlock()
	default:
		p.closeMu.Unlock()
		return Result{}, ErrQueueFull
	}

	select {
	case res := <-out:
		return res.res, res.err
	case <-timeoutCtx.Done():
		return Result{}, timeoutCtx.Err()
	}
}
