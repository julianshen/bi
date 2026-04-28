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

// Convert resolves relative image references against the current working
// directory (testdata layout). Production callers use ConvertWithBase.
func Convert(html []byte, opts Options) ([]byte, error) {
	return ConvertWithBase(html, opts, ".")
}

// ConvertWithBase resolves relative image references against base.
func ConvertWithBase(html []byte, opts Options, base string) ([]byte, error) {
	md, err := defaultConv.ConvertString(string(html))
	if err != nil {
		return nil, err
	}
	out := normaliseHeadings([]byte(md))
	out = applyImageMode(out, opts.Images, base)
	return out, nil
}
