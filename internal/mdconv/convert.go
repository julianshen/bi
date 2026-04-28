package mdconv

import (
	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
)

// Convert turns HTML into Markdown per opts.
func Convert(html []byte, opts Options) ([]byte, error) {
	md, err := htmltomarkdown.ConvertString(string(html))
	if err != nil {
		return nil, err
	}
	return normaliseHeadings([]byte(md)), nil
}
