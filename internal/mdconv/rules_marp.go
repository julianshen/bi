package mdconv

import (
	"bytes"
	"regexp"
)

// slideBreakRE matches a markdown horizontal rule on its own line.
// After injectMarpSlideBreaks rewrites LO's page-break markers as <hr/>,
// html-to-markdown renders them as "---", "***", or "* * *". Accept all.
var slideBreakRE = regexp.MustCompile(`(?m)^\s*(?:-{3,}|\*{3,}|(?:\*\s*){3,})\s*$`)

// pageBreakOpenTagRE matches any open tag whose style attribute contains
// `page-break-before:always`. LO's html filter emits these (commonly as
// `<h1 style="page-break-before:always; "></h1>`) between slides on
// .pptx/.odp exports. RE2 lacks backreferences so we can't pair the open
// tag with its matching close in one pattern; injectMarpSlideBreaks
// rewrites the open tag to `<hr/>` and lets html-to-markdown discard the
// orphaned (now empty) close tag — its parser is tolerant of that.
var pageBreakOpenTagRE = regexp.MustCompile(
	`<[a-zA-Z0-9]+\b[^>]*\bstyle="[^"]*page-break-before:\s*always[^"]*"[^>]*>`,
)

// injectMarpSlideBreaks rewrites LO's page-break-before markers to
// <hr/> so html-to-markdown emits a horizontal rule the marp splitter
// can detect. Must run before scrubLONoise (which strips style attrs).
func injectMarpSlideBreaks(html []byte) []byte {
	return pageBreakOpenTagRE.ReplaceAll(html, []byte("<hr/>"))
}

// applyMarp wraps a flat markdown body in Marp front-matter and replaces
// horizontal-rule slide breaks with normalised `---` separators (one
// blank line on each side). Empty segments — e.g. consecutive <hr/> in
// the source — are dropped so the output never contains adjacent
// separators.
func applyMarp(md []byte) []byte {
	indices := slideBreakRE.FindAllIndex(md, -1)
	var segments [][]byte
	cursor := 0
	for _, m := range indices {
		seg := bytes.TrimSpace(md[cursor:m[0]])
		if len(seg) > 0 {
			segments = append(segments, seg)
		}
		cursor = m[1]
	}
	tail := bytes.TrimSpace(md[cursor:])
	if len(tail) > 0 {
		segments = append(segments, tail)
	}

	var buf bytes.Buffer
	buf.WriteString("---\nmarp: true\n---\n\n")
	for i, seg := range segments {
		if i > 0 {
			buf.WriteString("\n\n---\n\n")
		}
		buf.Write(seg)
	}
	buf.WriteByte('\n')
	return buf.Bytes()
}
