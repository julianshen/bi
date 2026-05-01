package server

import "context"

// LogMeta carries conversion details from the handler back to the
// access-log middleware. A pointer is stored in context so the handler
// can populate it and the middleware can read it after ServeHTTP.
type LogMeta struct {
	Format      string
	Page        int
	TotalPages  int
	InBytes     int64
	OutBytes    int64
	QueueWaitMs int64
	ConvertMs   int64
}

type logMetaKey struct{}

// WithLogMeta injects a fresh *LogMeta into ctx.
func WithLogMeta(ctx context.Context) (context.Context, *LogMeta) {
	m := &LogMeta{}
	return context.WithValue(ctx, logMetaKey{}, m), m
}

// LogMetaFrom retrieves the *LogMeta from ctx, or nil if absent.
func LogMetaFrom(ctx context.Context) *LogMeta {
	m, _ := ctx.Value(logMetaKey{}).(*LogMeta)
	return m
}
