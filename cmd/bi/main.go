// Command bi is the office-document conversion HTTP service.
//
// Routes (planned — finalised in a separate brainstorming session):
//
//	POST /v1/convert/pdf       multipart upload → application/pdf
//	POST /v1/convert/png       multipart upload → image/png (per-page)
//	POST /v1/convert/markdown  multipart upload → text/markdown
//	POST /v1/thumbnail         multipart upload → image/png (low-DPI page 0)
//	GET  /healthz              real round-trip conversion of a fixture
package main

func main() {
	// Wiring (config.Load → worker.New → server.New → http.ListenAndServe)
	// lands in a follow-up commit alongside the first end-to-end test.
}
