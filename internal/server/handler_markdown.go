package server

import (
	"net/http"

	"github.com/julianshen/bi/internal/ocr"
	"github.com/julianshen/bi/internal/worker"
)

func (s *Server) convertMarkdown(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	imagesRaw := q.Get("images")
	var imageMode worker.MarkdownImageMode
	switch imagesRaw {
	case "", "embed":
		imageMode = worker.MarkdownImagesEmbed
	case "drop":
		imageMode = worker.MarkdownImagesDrop
	default:
		WriteProblem(w, r.URL.Path, RequestIDFrom(r.Context()),
			ErrBadQuery{Param: "images", Value: imagesRaw})
		return
	}

	ocrRaw := q.Get("ocr")
	var ocrMode worker.OCRMode
	switch ocrRaw {
	case "", "auto":
		ocrMode = worker.OCRAuto
	case "always":
		ocrMode = worker.OCRAlways
	case "never":
		ocrMode = worker.OCRNever
	default:
		WriteProblem(w, r.URL.Path, RequestIDFrom(r.Context()),
			ErrBadQuery{Param: "ocr", Value: ocrRaw})
		return
	}

	ocrLang := q.Get("ocr_lang")
	if ocrLang == "" {
		ocrLang = ocr.LangAuto
	}
	if err := ocr.ValidateLangs(ocrLang); err != nil {
		WriteProblem(w, r.URL.Path, RequestIDFrom(r.Context()),
			ErrBadQuery{Param: "ocr_lang", Value: ocrLang})
		return
	}

	if ocrMode == worker.OCRAlways && !s.deps.OCRAvailable {
		WriteProblem(w, r.URL.Path, RequestIDFrom(r.Context()),
			worker.ErrOCRUnavailable)
		return
	}

	s.handleConversion(w, r, worker.Job{
		Format:         worker.FormatMarkdown,
		MarkdownImages: imageMode,
		MarkdownMarp:   isPresentationContentType(r.Header.Get("Content-Type")),
		Password:       r.Header.Get("X-Bi-Password"),
		OCRMode:        ocrMode,
		OCRLang:        ocrLang,
	})
}
