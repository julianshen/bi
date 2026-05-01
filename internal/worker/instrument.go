package worker

import (
	"context"
	"time"
)

// Instrumenter is the hook surface the worker uses to report queue,
// conversion, and error telemetry. The server package provides an
// implementation that forwards to Prometheus.
type Instrumenter interface {
	QueueWait(format Format, d time.Duration)
	ConversionDuration(format Format, d time.Duration)
	QueueDepth(d int)
	WorkerBusy(delta int)
	LokError(kind string)
}

// Timing carries millisecond-resolution breakdown of a conversion
// back to the caller (typically the HTTP handler for access-log
// enrichment). Pass a non-nil *Timing via WithTiming.
type Timing struct {
	QueueWaitMs int64
	ConvertMs   int64
}

type timingKey struct{}

// WithTiming attaches a *Timing to ctx so Pool.Run can populate it.
func WithTiming(ctx context.Context, t *Timing) context.Context {
	return context.WithValue(ctx, timingKey{}, t)
}

// TimingFrom retrieves the *Timing previously attached by WithTiming.
func TimingFrom(ctx context.Context) *Timing {
	t, _ := ctx.Value(timingKey{}).(*Timing)
	return t
}
