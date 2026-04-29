package server

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/julianshen/bi/internal/worker"
)

func (s *Server) convertPDF(w http.ResponseWriter, r *http.Request) {
	s.handleConversion(w, r, worker.Job{
		Format:   worker.FormatPDF,
		Password: r.Header.Get("X-Bi-Password"),
	})
}

// handleConversion is the shared body-capture + worker-dispatch + response-stream
// pipeline used by every conversion handler. Job-shape decisions belong to the
// caller, communicated via build().
func (s *Server) handleConversion(w http.ResponseWriter, r *http.Request, job worker.Job) {
	ct := r.Header.Get("Content-Type")
	if ct == "" {
		WriteProblem(w, r.URL.Path, RequestIDFrom(r.Context()), ErrMissingContentType)
		return
	}
	// LibreOffice refuses to load files without a recognisable extension
	// ("Unspecified Application Error" on stderr). Map Content-Type to
	// extension so the temp filename gives LO the format hint it needs.
	tmp, err := os.CreateTemp("", "bi-in-*"+extensionFromContentType(ct))
	if err != nil {
		WriteProblem(w, r.URL.Path, RequestIDFrom(r.Context()), err)
		return
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, r.Body); err != nil {
		tmp.Close()
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			WriteProblem(w, r.URL.Path, RequestIDFrom(r.Context()), ErrPayloadTooLarge)
			return
		}
		WriteProblem(w, r.URL.Path, RequestIDFrom(r.Context()), err)
		return
	}
	tmp.Close()

	job.InPath = tmp.Name()
	res, err := s.deps.Conv.Run(r.Context(), job)
	if err != nil {
		WriteProblem(w, r.URL.Path, RequestIDFrom(r.Context()), err)
		return
	}
	defer os.Remove(res.OutPath)

	// Buffer the output through []byte rather than streaming via os.Open
	// + io.Copy. With LO 25.x in this image, a streamed response from the
	// just-written file occasionally panics LO with "Unspecified Application
	// Error" — likely an inotify/file-handle interaction with LO's still-
	// open output. ReadFile-then-Write avoids it. Outputs are bounded by
	// MaxUploadBytes anyway.
	body, err := os.ReadFile(res.OutPath)
	if err != nil {
		WriteProblem(w, r.URL.Path, RequestIDFrom(r.Context()), err)
		return
	}
	w.Header().Set("Content-Type", res.MIME)
	if res.TotalPages > 0 {
		w.Header().Set("X-Total-Pages", strconv.Itoa(res.TotalPages))
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// extensionFromContentType returns a leading-dot extension that LibreOffice
// can use as a format hint, or empty string if we can't classify it (LO will
// then fail with a clear error rather than crashing). Office formats that
// LO knows but mime.ExtensionsByType doesn't are mapped explicitly.
func extensionFromContentType(ct string) string {
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return ""
	}
	switch mt {
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return ".docx"
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return ".xlsx"
	case "application/vnd.openxmlformats-officedocument.presentationml.presentation":
		return ".pptx"
	case "application/vnd.oasis.opendocument.text":
		return ".odt"
	case "application/vnd.oasis.opendocument.spreadsheet":
		return ".ods"
	case "application/vnd.oasis.opendocument.presentation":
		return ".odp"
	case "application/vnd.oasis.opendocument.graphics":
		return ".odg"
	case "application/msword":
		return ".doc"
	case "application/vnd.ms-excel":
		return ".xls"
	case "application/vnd.ms-powerpoint":
		return ".ppt"
	case "application/rtf", "text/rtf":
		return ".rtf"
	case "text/plain":
		return ".txt"
	case "text/html":
		return ".html"
	case "text/csv":
		return ".csv"
	}
	if exts, _ := mime.ExtensionsByType(ct); len(exts) > 0 {
		return exts[0]
	}
	// Fall back to a generic ".bin" so the file at least has *some*
	// extension; LO will still produce a meaningful error if it really
	// can't parse the content.
	if strings.HasPrefix(mt, "application/") || strings.HasPrefix(mt, "text/") {
		return ".bin"
	}
	return ""
}
