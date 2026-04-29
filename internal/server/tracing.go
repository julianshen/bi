package server

import (
	"context"
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// InitTracing wires an OTLP/gRPC exporter using standard OTEL_* env and
// returns a shutdown func to call at exit. Per-request spans come from
// otelhttp middleware automatically — explicit Tracer wiring will land
// when the worker introduces queue.wait / lok.* spans.
func InitTracing(ctx context.Context, serviceName string) (func(context.Context) error, error) {
	exp, err := otlptracegrpc.New(ctx)
	if err != nil {
		return nil, err
	}
	res, _ := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(serviceName)),
		resource.WithFromEnv(),
	)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	return tp.Shutdown, nil
}

// otelMiddleware wraps a handler so each request gets an http.server.request
// span automatically.
func otelMiddleware(next http.Handler) http.Handler {
	return otelhttp.NewHandler(next, "bi.http", otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
		return r.Method + " " + r.URL.Path
	}))
}
