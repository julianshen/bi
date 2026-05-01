package server

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"strconv"

	"github.com/julianshen/bi/internal/worker"
)

func (s *Server) convertPDF(w http.ResponseWriter, r *http.Request) {
	if isPDFContentType(r.Header.Get("Content-Type")) {
		WriteProblem(w, r.URL.Path, RequestIDFrom(r.Context()), ErrPDFNotAcceptedAsInput)
		return
	}
	s.handleConversion(w, r, worker.Job{
		Format:   worker.FormatPDF,
		Password: r.Header.Get("X-Bi-Password"),
	})
}

// handleConversion is the shared body-capture + worker-dispatch + response-stream
// pipeline used by every conversion handler. The caller passes a partly-built
// worker.Job (Format and any per-format options); handleConversion fills InPath
// after staging the request body.
func (s *Server) handleConversion(w http.ResponseWriter, r *http.Request, job worker.Job) {
	ctx, span := tracer.Start(r.Context(), "convert."+job.Format.String())
	defer span.End()

	meta := LogMetaFrom(ctx)
	if meta != nil {
		meta.Format = job.Format.String()
		meta.Page = job.Page
	}

	ct := r.Header.Get("Content-Type")
	if ct == "" {
		WriteProblem(w, r.URL.Path, RequestIDFrom(ctx), ErrMissingContentType)
		return
	}
	// LibreOffice refuses to load files without a recognisable extension
	// ("Unspecified Application Error" on stderr). Map Content-Type to
	// extension so the temp filename gives LO the format hint it needs.
	tmp, err := os.CreateTemp("", "bi-in-*"+extensionFromContentType(ct))
	if err != nil {
		WriteProblem(w, r.URL.Path, RequestIDFrom(ctx), err)
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		tmp.Close()
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			WriteProblem(w, r.URL.Path, RequestIDFrom(ctx), ErrPayloadTooLarge)
			return
		}
		WriteProblem(w, r.URL.Path, RequestIDFrom(ctx), err)
		return
	}
	if meta != nil {
		meta.InBytes = int64(len(body))
	}
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		WriteProblem(w, r.URL.Path, RequestIDFrom(ctx), err)
		return
	}
	tmp.Close()

	job.InPath = tmpName
	timing := &worker.Timing{}
	ctx = worker.WithTiming(ctx, timing)
	res, err := s.deps.Conv.Run(ctx, job)
	if err != nil {
		WriteProblem(w, r.URL.Path, RequestIDFrom(r.Context()), err)
		return
	}
	defer os.Remove(res.OutPath)
	if meta != nil {
		meta.QueueWaitMs = timing.QueueWaitMs
		meta.ConvertMs = timing.ConvertMs
		meta.TotalPages = res.TotalPages
	}

	f, err := os.Open(res.OutPath)
	if err != nil {
		WriteProblem(w, r.URL.Path, RequestIDFrom(r.Context()), err)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		WriteProblem(w, r.URL.Path, RequestIDFrom(r.Context()), err)
		return
	}
	if meta != nil {
		meta.OutBytes = info.Size()
	}
	w.Header().Set("Content-Type", res.MIME)
	if res.TotalPages > 0 {
		w.Header().Set("X-Total-Pages", strconv.Itoa(res.TotalPages))
	}
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	w.WriteHeader(http.StatusOK)
	ctx, wspan := tracer.Start(r.Context(), "response.write")
	_, err = io.Copy(w, f)
	wspan.End()
	if err != nil {
		// Status was already written; we can't change it. Log so a stream
		// truncation (client disconnect, disk read error, ResponseWriter
		// failure) is observable instead of silent.
		s.deps.Logger.WarnContext(r.Context(), "stream response",
			"err", err, "path", r.URL.Path)
	}
	_ = ctx
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
	case "application/pdf":
		return ".pdf"
	case "application/rtf", "text/rtf":
		return ".rtf"
	case "text/plain":
		return ".txt"
	case "text/html":
		return ".html"
	case "text/csv":
		return ".csv"
	}
	if exts, _ := mime.ExtensionsByType(mt); len(exts) > 0 {
		return exts[0]
	}
	return ""
}

// isPresentationContentType reports whether ct identifies a presentation
// format LO recognises. Used by the markdown handler to auto-enable Marp
// output for pptx/odp/ppt uploads.
func isPresentationContentType(ct string) bool {
	switch extensionFromContentType(ct) {
	case ".pptx", ".odp", ".ppt":
		return true
	}
	return false
}

// isPDFContentType reports whether ct identifies a PDF body.
func isPDFContentType(ct string) bool {
	return extensionFromContentType(ct) == ".pdf"
}
