package mdconv

import (
	"bytes"
	"regexp"
)

// slideBreakRE matches a markdown horizontal rule on its own line.
// After injectMarpSlideBreaks rewrites LO's page-break markers as <hr/>,
// html-to-markdown currently emits "---", "***", or "* * *" depending
// on context. The pattern accepts the family rather than a closed list
// so a library upgrade adding another HR style doesn't silently drop
// slide splits — the cases are pinned in TestApplyMarpAcceptsAllHRForms.
var slideBreakRE = regexp.MustCompile(`(?m)^\s*(?:-{3,}|\*{3,}|(?:\*\s*){3,})\s*$`)

// pageBreakOpenTagRE matches any open tag whose double-quoted style
// attribute contains `page-break-before:always`. LO's html filter emits
// these between slides on .pptx/.odp exports — tag name varies (we've
// observed h1 and div); style position and surrounding properties also
// vary. RE2 lacks backreferences so we can't pair the open tag with its
// matching close in one pattern; injectMarpSlideBreaks rewrites the open
// tag to `<hr/>` and relies on html-to-markdown to drop the orphaned
// close tag (covered by the marp-lo-pagebreak fixture). Single-quoted
// styles aren't handled — LO has only been observed emitting double
// quotes; widen this if that changes.
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
	const frontMatter = "---\nmarp: true\n---\n\n"
	const sep = "\n\n---\n\n"

	var buf bytes.Buffer
	buf.Grow(len(md) + len(frontMatter) + 1)
	buf.WriteString(frontMatter)

	cursor := 0
	// `wrote` suppresses a leading separator when early segments are
	// empty (e.g. an HR at the very top of the body). Without it, an
	// empty first segment followed by content would produce a deck
	// starting with a stray "---" between front-matter and slide one.
	wrote := false
	emit := func(seg []byte) {
		if len(seg) == 0 {
			return
		}
		if wrote {
			buf.WriteString(sep)
		}
		buf.Write(seg)
		wrote = true
	}
	for _, m := range slideBreakRE.FindAllIndex(md, -1) {
		emit(bytes.TrimSpace(md[cursor:m[0]]))
		cursor = m[1]
	}
	emit(bytes.TrimSpace(md[cursor:]))

	buf.WriteByte('\n')
	return buf.Bytes()
}
