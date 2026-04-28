package worker

// Compile-time assertions that the fakes satisfy the interfaces.
var (
	_ lokOffice   = (*fakeOffice)(nil)
	_ lokDocument = (*fakeDocument)(nil)
)

// fakeOffice records calls and returns scripted outcomes.
type fakeOffice struct {
	loadCalls  []string
	loadErr    error
	loadDoc    *fakeDocument
	closeCalls int
	closeErr   error
}

func (f *fakeOffice) Load(path, password string) (lokDocument, error) {
	f.loadCalls = append(f.loadCalls, path)
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	if f.loadDoc == nil {
		f.loadDoc = &fakeDocument{parts: 1}
	}
	return f.loadDoc, nil
}

func (f *fakeOffice) Close() error { f.closeCalls++; return f.closeErr }

type fakeDocument struct {
	parts        int
	saveAsCalls  []saveAsCall
	saveAsErr    error
	saveAsHook   func(path, filter, options string) error // optional, runs after recording
	renderErr    error
	renderBytes  []byte
	closeCalls   int
}

type saveAsCall struct{ Path, Filter, Options string }

func (f *fakeDocument) SaveAs(path, filter, options string) error {
	f.saveAsCalls = append(f.saveAsCalls, saveAsCall{path, filter, options})
	if f.saveAsErr != nil {
		return f.saveAsErr
	}
	if f.saveAsHook != nil {
		return f.saveAsHook(path, filter, options)
	}
	return nil
}
func (f *fakeDocument) InitializeForRendering(arg string) error { return nil }
func (f *fakeDocument) RenderPagePNG(page int, dpi float64) ([]byte, error) {
	if f.renderErr != nil {
		return nil, f.renderErr
	}
	if f.renderBytes != nil {
		return f.renderBytes, nil
	}
	return []byte("fake-png"), nil
}
func (f *fakeDocument) GetParts() int { return f.parts }
func (f *fakeDocument) Close() error  { f.closeCalls++; return nil }
