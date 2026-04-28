package server

import (
	"net/http"

	"github.com/julianshen/bi/internal/worker"
)

func (s *Server) convertMarkdown(w http.ResponseWriter, r *http.Request) {
	mode := worker.MarkdownImagesEmbed
	switch r.URL.Query().Get("images") {
	case "", "embed":
		mode = worker.MarkdownImagesEmbed
	case "drop":
		mode = worker.MarkdownImagesDrop
	default:
		WriteProblem(w, r.URL.Path, RequestIDFrom(r.Context()),
			ErrBadQuery{Param: "images", Value: r.URL.Query().Get("images")})
		return
	}
	s.handleConversion(w, r, func() worker.Job {
		return worker.Job{
			Format:         worker.FormatMarkdown,
			MarkdownImages: mode,
			Password:       r.Header.Get("X-Bi-Password"),
		}
	})
}
