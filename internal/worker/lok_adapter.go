//go:build !nolok

package worker

import (
	"os"

	"github.com/julianshen/golibreofficekit/lok"
)

// newRealOffice initialises lok.Office and wraps it as a lokOffice.
func newRealOffice(path string) (lokOffice, error) {
	off, err := lok.New(path)
	if err != nil {
		return nil, wrapIfLOK(err)
	}
	return realOffice{off: off}, nil
}

type realOffice struct{ off *lok.Office }

func (o realOffice) Load(path, password string) (lokDocument, error) {
	var opts []lok.LoadOption
	if password != "" {
		opts = append(opts, lok.WithPassword(password))
	}
	doc, err := o.off.Load(path, opts...)
	if err != nil {
		return nil, wrapIfLOK(err)
	}
	return realDoc{doc: doc}, nil
}

func (o realOffice) Close() error { return o.off.Close() }

type realDoc struct{ doc *lok.Document }

func (d realDoc) SaveAs(path, filter, options string) error {
	return wrapIfLOK(d.doc.SaveAs(path, filter, options))
}
func (d realDoc) InitializeForRendering(arg string) error {
	return wrapIfLOK(d.doc.InitializeForRendering(arg))
}
func (d realDoc) RenderPagePNG(page int, dpi float64) ([]byte, error) {
	b, err := d.doc.RenderPagePNG(page, dpi)
	return b, wrapIfLOK(err)
}
func (d realDoc) GetParts() (int, error) {
	n, err := d.doc.Parts()
	return n, wrapIfLOK(err)
}
func (d realDoc) Close() error { return d.doc.Close() }

// wrapIfLOK marks errors from lok with the LOK() interface used by Classify.
func wrapIfLOK(err error) error {
	if err == nil {
		return nil
	}
	return lokErrWrap{err}
}

type lokErrWrap struct{ err error }

func (e lokErrWrap) Error() string { return e.err.Error() }
func (e lokErrWrap) Unwrap() error { return e.err }
func (e lokErrWrap) LOK() bool     { return true }

func removeQuiet(path string) error { return os.Remove(path) }
