package server

import (
	"net/http"

	"github.com/julianshen/bi/internal/worker"
)

func (s *Server) convertMarkdown(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("images")
	var mode worker.MarkdownImageMode
	switch raw {
	case "", "embed":
		mode = worker.MarkdownImagesEmbed
	case "drop":
		mode = worker.MarkdownImagesDrop
	default:
		WriteProblem(w, r.URL.Path, RequestIDFrom(r.Context()),
			ErrBadQuery{Param: "images", Value: raw})
		return
	}
	s.handleConversion(w, r, worker.Job{
		Format:         worker.FormatMarkdown,
		MarkdownImages: mode,
		MarkdownMarp:   isPresentationContentType(r.Header.Get("Content-Type")),
		Password:       r.Header.Get("X-Bi-Password"),
	})
}
