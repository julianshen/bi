package mdconv

import (
	"bytes"
	"regexp"
	"strings"
)

var headingRE = regexp.MustCompile(`(?m)^(#{1,6})\s`)

// normaliseHeadings rebases the highest heading level to # so the output is
// independent of where LO chose to start.
func normaliseHeadings(md []byte) []byte {
	matches := headingRE.FindAllSubmatch(md, -1)
	if len(matches) == 0 {
		return md
	}
	minLevel := 6
	for _, m := range matches {
		if l := len(m[1]); l < minLevel {
			minLevel = l
		}
	}
	if minLevel == 1 {
		return md
	}
	delta := minLevel - 1
	return headingRE.ReplaceAllFunc(md, func(b []byte) []byte {
		hashes := bytes.IndexByte(b, ' ')
		return []byte(strings.Repeat("#", hashes-delta) + " ")
	})
}
