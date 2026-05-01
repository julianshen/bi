# PDF as input format — Design

**Status:** Implemented 2026-05-01 with mid-flight architecture pivot.
**Scope:** Accept `application/pdf` uploads on `/v1/convert/png` and
`/v1/convert/markdown`. Reject on `/v1/convert/pdf`.

**Engine split (post-implementation note):** LibreOffice's pdfimport
extension was the planned engine for both routes, but it flattens PDF
pages to embedded images during load — the existing pdf → SaveAs("html")
→ mdconv pipeline returned empty markdown. Per-route engines now:

- **PNG:** LibreOffice (still loads PDFs as Draw documents and
  re-rasterises the page to PNG). Runtime image switched from Debian
  bookworm to Ubuntu 24.04 because Debian's `libreoffice-pdfimport`
  package is no longer published.
- **Markdown:** `github.com/ledongthuc/pdf` (BSD-3, fork of
  `rsc.io/pdf`) extracts text directly. The handler stages the PDF to
  a temp file with `.pdf` suffix; `runMarkdown` short-circuits before
  `office.Load` based on that extension.

Scanned/image-only PDFs still produce empty markdown — the limitation
documented under non-goals just has a different proximate cause.

## Goals

- `POST /v1/convert/png` with a PDF body renders a page to PNG.
- `POST /v1/convert/markdown` with a PDF body extracts text/layout via
  the existing pdf → html → markdown pipeline.
- `POST /v1/convert/pdf` with a PDF body returns 415 — the route is
  "convert *to* PDF", not "passthrough/re-export".
- Auto-detected from request `Content-Type`. No new endpoint, no new
  flag, no breaking changes.

## Non-goals (v1)

- OCR for scanned/image-only PDFs. They will produce empty or
  near-empty markdown; documented as a known limitation.
- A passthrough or re-export path on `/v1/convert/pdf`.
- A separate `/v1/convert/text` endpoint for PDF text extraction —
  callers use `/v1/convert/markdown`.
- Form field extraction, JavaScript execution, annotations.
- Migrating to Poppler (`pdftoppm` / `pdftotext`). Tracked as a
  possible future change once we have real-usage signal on LO's
  fidelity. The migration boundary is documented in §Migration below.

## Detection

`internal/server/handler_pdf.go:extensionFromContentType` gets one new
explicit case:

| Content-Type | Extension |
|---|---|
| `application/pdf` | `.pdf` |

A new helper `isPDFContentType(ct string) bool` lives next to
`isPresentationContentType`, sharing the same delegation pattern:

```go
func isPDFContentType(ct string) bool {
    return extensionFromContentType(ct) == ".pdf"
}
```

The PDF handler (`/v1/convert/pdf`) calls this helper at the start of
its handler and returns 415 with a problem document if it matches.
Other handlers don't change — they already accept any LO-loadable
input.

## Architecture

The pipeline does not change shape:

```
HTTP handler  ──►  worker.Pool  ──►  lok.Office.Load(pdf)  ──►  Draw doc
                                     │
                                     ├─ run_png:      InitializeForRendering + RenderPagePNG
                                     └─ run_markdown: SaveAs(html) ──► mdconv
```

`libreoffice-pdfimport` registers as an LO filter, so `Load` accepts
`.pdf` inputs and returns a Draw document that the existing `run_png`
and `run_markdown` consume without modification. PDF → markdown reuses
the entire mdconv pipeline (`scrubLONoise`, `defaultConv.ConvertString`,
`normaliseHeadings`, `applyImageMode`); `applyMarp` is **not** applied
to PDFs because PDFs aren't presentations.

## Components

### New
- `internal/server/handler_pdf.go:isPDFContentType` (mirrors
  `isPresentationContentType`).
- `internal/server/handler_pdf.go:extensionFromContentType` —
  `application/pdf` case added to the existing switch.
- `internal/server/handler_pdf.go` handler — early 415 return when
  request Content-Type matches PDF.
- `testdata/health.pdf` — small fixture (a few KB) containing the
  literal text "Hello PDF" for integration tests to verify.

### Modified
- `Dockerfile` — add `libreoffice-pdfimport` to the apt install list.
- `Dockerfile.test` — same.
- `internal/server/handler_pdf_extension_test.go` — add
  `application/pdf` case to the existing extension table.
- `internal/server/handler_pdf_test.go` — new test for the 415 reject
  on `/v1/convert/pdf` when Content-Type is `application/pdf`.
- `internal/server/handler_png_test.go` — confirms PNG handler accepts
  PDF Content-Type and dispatches a job (via fakeConverter).
- `internal/server/handler_markdown_test.go` — same for markdown.
- `internal/worker/integration_test.go` — `PDFInputPNG` and
  `PDFInputMarkdown` subtests under `TestRealConversion`, sharing the
  existing Pool (lok-init constraint).
- `docs/superpowers/specs/2026-04-28-bi-http-api-design.md` — note
  that PDF is a valid input on PNG/markdown routes; rejected on PDF
  route.

## Error model

No new sentinels.

| Failure | Source | Sentinel | HTTP |
|---|---|---|---|
| Malformed PDF | lok rejects in `Load` | `ErrUnsupportedFormat` | 415 |
| Encrypted PDF without password | lok | `ErrPasswordRequired` | 401 |
| Wrong password | lok | `ErrWrongPassword` | 401 |
| `/v1/convert/pdf` + PDF body | handler | new explicit 415 path | 415 |
| Scanned PDF (no text layer) | LO's pdfimport | (none — succeeds with empty body) | 200 |

The "scanned PDF → empty markdown" case is intentional v1 behavior, not
an error. Callers who need OCR run a separate pipeline upstream.

## Testing

### Unit (server)
- `extensionFromContentType("application/pdf") == ".pdf"` — added to
  the existing table-driven test.
- `isPDFContentType` parity with the existing
  `isPresentationContentType` test pattern.
- `POST /v1/convert/pdf` with `Content-Type: application/pdf` returns
  415 (no upstream Pool dispatch).
- `POST /v1/convert/png` with `Content-Type: application/pdf`
  dispatches a job with `Format: FormatPNG` (verified via
  `fakeConverter`).
- `POST /v1/convert/markdown` with `Content-Type: application/pdf`
  dispatches a job with `Format: FormatMarkdown, MarkdownMarp: false`.

### Integration (worker, build tag `integration`)
Two new subtests under `TestRealConversion`:

- `PDFInputPNG`: `worker.Job{InPath: testdata/health.pdf, Format:
  FormatPNG, Page: 0, DPI: 1.0}` → expect `MIME: "image/png"`, body
  starts with PNG magic, body length > 100.
- `PDFInputMarkdown`: `worker.Job{InPath: testdata/health.pdf,
  Format: FormatMarkdown, MarkdownImages: MarkdownImagesEmbed}` →
  expect `MIME: "text/markdown"`, body contains "Hello PDF".

Both subtests use the existing shared Pool — adding a third Pool would
re-trigger the lok-init-once trap.

### Smoke / coverage gates
Per-package coverage thresholds unchanged. The handler change is one
switch case + one helper + one early-return — covered by the unit
tests above. Coverage gates hold without further work.

## Migration to Poppler (future)

The swap point is `internal/worker/run_png.go` and
`internal/worker/run_markdown.go`. To migrate from LO pdfimport to
Poppler:

1. Replace `libreoffice-pdfimport` with `poppler-utils` in both
   Dockerfiles.
2. At the top of each runner, branch on input extension: if `.pdf`,
   exec `pdftoppm` (PNG) or `pdftotext` (markdown) into a temp file
   instead of going through `doc.Load` + `SaveAs`.
3. Either keep the existing `mdconv` post-processing for PDF→markdown
   (run pdftotext output through it) or skip it and ship pdftotext's
   raw output directly. Decide at migration time based on observed
   quality.
4. Tests stay; only the engine binding changes.

The handler layer is engine-agnostic and does not change. No interface
abstraction is added today (YAGNI) — when migration happens, the
engineer introduces it as part of that PR with a clear before/after
diff.

## Acceptance criteria

The implementation is complete when:

1. `POST /v1/convert/png` with a `.pdf` body returns 200 with
   `Content-Type: image/png` and a valid PNG body.
2. `POST /v1/convert/markdown` with a `.pdf` body returns 200 with
   `Content-Type: text/markdown` and a body containing the fixture
   text.
3. `POST /v1/convert/pdf` with a `.pdf` body returns 415.
4. `make cover-gate` passes, `make docker-test` passes (now exercising
   PDF→PNG and PDF→Markdown subtests against real LO).
5. Runtime image (`Dockerfile`) has `libreoffice-pdfimport` installed;
   confirmed by inspecting the built image apt manifest.
