package worker

import "go.opentelemetry.io/otel"

var tracer = otel.Tracer("github.com/julianshen/bi/internal/worker")
