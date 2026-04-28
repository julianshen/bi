package mdconv

import (
	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
)

var defaultConv = converter.NewConverter(
	converter.WithPlugins(
		base.NewBasePlugin(),
		commonmark.NewCommonmarkPlugin(),
		table.NewTablePlugin(),
	),
)

// Convert turns HTML into Markdown per opts.
func Convert(html []byte, opts Options) ([]byte, error) {
	md, err := defaultConv.ConvertString(string(html))
	if err != nil {
		return nil, err
	}
	return normaliseHeadings([]byte(md)), nil
}
