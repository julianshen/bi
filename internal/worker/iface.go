package worker

// lokOffice is a process singleton mirroring the subset of *lok.Office that
// the worker uses. The unexported name keeps it private to this package; the
// real adapter and the test fake both satisfy it.
type lokOffice interface {
	Load(path, password string) (lokDocument, error)
	Close() error
}

// lokDocument mirrors the subset of *lok.Document used by the worker.
type lokDocument interface {
	SaveAs(path, filter, options string) error
	InitializeForRendering(arg string) error
	RenderPagePNG(page int, dpi float64) ([]byte, error)
	GetParts() int
	Close() error
}
