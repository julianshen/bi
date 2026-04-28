package mdconv

import "regexp"

var (
	styleAttrRE = regexp.MustCompile(`\s+style="[^"]*"`)
	fontTagRE   = regexp.MustCompile(`</?font[^>]*>`)
	classAttrRE = regexp.MustCompile(`\s+class="[^"]*"`)
)

func scrubLONoise(html []byte) []byte {
	html = styleAttrRE.ReplaceAll(html, nil)
	html = fontTagRE.ReplaceAll(html, nil)
	html = classAttrRE.ReplaceAll(html, nil)
	return html
}
