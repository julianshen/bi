package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/julianshen/bi/internal/worker"
)

type Problem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Detail    string `json:"detail,omitempty"`
	Instance  string `json:"instance"`
	RequestID string `json:"request_id,omitempty"`
}

// Server-side sentinels that propagate to the problem mapper.
var (
	ErrMissingContentType    = errors.New("missing Content-Type")
	ErrPayloadTooLarge       = errors.New("payload too large")
	ErrPDFNotAcceptedAsInput = errors.New("PDF input is not accepted on the PDF route; use /v1/convert/png or /v1/convert/markdown")
)

type problemMapping struct {
	slug   string
	title  string
	status int
}

func mapError(err error) problemMapping {
	switch {
	case errors.Is(err, worker.ErrQueueFull):
		return problemMapping{"queue-full", "Server busy", http.StatusTooManyRequests}
	case errors.Is(err, worker.ErrPoolClosed):
		return problemMapping{"shutting-down", "Service shutting down", http.StatusServiceUnavailable}
	case errors.Is(err, worker.ErrPasswordRequired):
		return problemMapping{"password-required", "Password required", http.StatusUnprocessableEntity}
	case errors.Is(err, worker.ErrWrongPassword):
		return problemMapping{"password-wrong", "Wrong password", http.StatusUnprocessableEntity}
	case errors.Is(err, worker.ErrUnsupportedFormat):
		return problemMapping{"unsupported-document", "Unsupported document", http.StatusUnprocessableEntity}
	case errors.Is(err, worker.ErrMarkdownConversion):
		return problemMapping{"markdown-pipeline", "Markdown rendering failed", http.StatusInternalServerError}
	case errors.Is(err, worker.ErrLOKUnsupported):
		return problemMapping{"lok-unsupported", "LibreOffice build is missing required functionality", http.StatusNotImplemented}
	case errors.Is(err, worker.ErrOCRUnavailable):
		return problemMapping{"ocr-unavailable", "OCR not available", http.StatusServiceUnavailable}
	case errors.Is(err, worker.ErrOCRFailed):
		return problemMapping{"ocr-failed", "OCR pipeline failed", http.StatusBadGateway}
	case errors.Is(err, worker.ErrPageOutOfRange), errors.Is(err, worker.ErrInvalidDPI):
		return problemMapping{"bad-request", "Bad request", http.StatusBadRequest}
	case errors.Is(err, ErrMissingContentType):
		return problemMapping{"unsupported-media-type", "Content-Type required", http.StatusUnsupportedMediaType}
	case errors.Is(err, ErrPDFNotAcceptedAsInput):
		return problemMapping{"unsupported-media-type", "PDF input not accepted on this route", http.StatusUnsupportedMediaType}
	case errors.Is(err, ErrPayloadTooLarge):
		return problemMapping{"payload-too-large", "Payload too large", http.StatusRequestEntityTooLarge}
	case errors.Is(err, context.DeadlineExceeded):
		return problemMapping{"timeout", "Conversion timed out", http.StatusGatewayTimeout}
	case errors.As(err, new(ErrBadQuery)):
		return problemMapping{"bad-request", "Bad query parameter", http.StatusBadRequest}
	default:
		return problemMapping{"internal", "Internal server error", http.StatusInternalServerError}
	}
}

// ErrBadQuery surfaces invalid query parameters.
type ErrBadQuery struct{ Param, Value string }

func (e ErrBadQuery) Error() string { return "bad query " + e.Param + "=" + e.Value }

// WriteProblem renders an RFC 7807 response.
func WriteProblem(w http.ResponseWriter, instance, requestID string, err error) {
	m := mapError(err)
	if m.slug == "queue-full" {
		w.Header().Set("Retry-After", "1")
	}
	p := Problem{
		Type:      "https://bi/errors/" + m.slug,
		Title:     m.title,
		Status:    m.status,
		Detail:    err.Error(),
		Instance:  instance,
		RequestID: requestID,
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(m.status)
	_ = json.NewEncoder(w).Encode(p)
}
