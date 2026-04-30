package mdconv

import (
	"bytes"
	"regexp"
)

// slideBreakRE matches a markdown horizontal rule on its own line.
// LO emits <hr/> between slides on .pptx/.odp HTML export; html-to-markdown
// renders that as either "---" or "* * *" (or "***"). We accept all three.
var slideBreakRE = regexp.MustCompile(`(?m)^\s*(?:-{3,}|\*{3,}|(?:\*\s*){3,})\s*$`)

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
