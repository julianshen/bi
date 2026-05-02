# OCR for scanned PDFs in the markdown route

**Date:** 2026-05-02
**Status:** Approved (brainstorming complete; awaiting implementation plan)
**Branch:** `feat/ocr`

## Goal

Augment the existing `POST /markdown` route so that PDF inputs whose
pages have no usable text layer (typical of scans) yield meaningful
markdown via OCR. Supported scripts: English, Japanese, Simplified
Chinese, Traditional Chinese.

OCR is **not** a new endpoint, **not** a new conversion mode for
office documents, and **not** an asynchronous job. It is a per-page
fallback inside the markdown PDF pipeline.

## Non-goals (v1)

- OCR for non-PDF inputs (`.docx`, `.pptx`, …).
- Layout / structure recovery (headings, tables, columns).
- Per-token confidence scores in output.
- Cloud OCR or PaddleOCR backends. The interface allows them later;
  no implementation in this milestone.
- Async / job-queue API. The existing sync request model continues.
  Long PDFs are bounded by the existing convert timeout.

## API surface

`POST /markdown` (PDF input) gains two optional query params:

| Param      | Values                                                                 | Default  |
|------------|------------------------------------------------------------------------|----------|
| `ocr`      | `auto` \| `always` \| `never`                                          | `auto`   |
| `ocr_lang` | `eng`, `jpn`, `chi_sim`, `chi_tra`, `+`-joined (e.g. `eng+jpn`), `auto`, `all` | `auto`   |

`auto` (lang) means: detect script per page using Tesseract OSD, then
recognize with the matching single language.

`all` (lang) is a shorthand for `eng+jpn+chi_sim+chi_tra` — all four
language packs in one Tesseract pass.

A page is OCR'd when:

- `ocr=always`, OR
- `ocr=auto` AND `len(strings.TrimSpace(extractedText)) < ocrTextThreshold`
  (default 16, env-configurable).

`ocr=never` disables OCR even if extraction yields nothing.

### Validation

- `ocr` value not in the allowlist → 400.
- `ocr_lang` value not in the allowlist (or contains an unknown
  component in a `+`-joined value) → 400.
- `ocr=always|auto` but the OCR engine is not configured at startup
  → 503 with problem `code=ocr_unavailable`.

## Architecture

```
POST /markdown (PDF)            HTTP server (parent process — pure Go)
        │
        ├── parse + validate ocr / ocr_lang query params
        │
        ▼
  SubprocessConverter spawns:  bi convert -in ... -ocr ... -ocr-lang ...
        │
        ▼
  bi convert (child process — cgo for both lok and OCR)
        │
        ▼
  worker.Pool (Workers=1 in subprocess; owns *lok.Office + ocr.Engine)
        │
        ├── extractPDFText(path)              ── existing per-page
        │
        ├── for each page that needs OCR:
        │       lokDoc.RenderPagePNG(i, 300)  ── existing render path
        │       ocrEngine.Recognize(png, lang)── new
        │
        └── assemble markdown:
              text pages as today,
              OCR pages with `<!-- ocr: <lang> page=N -->` marker,
              `---` page separator between pages.
```

OCR lives in the **child `bi convert` process**, alongside lok. The
parent serve process stays pure Go (no cgo, no Tesseract dependency
at link time). The child process is short-lived (one conversion);
`ocr.Engine` is constructed once per subprocess run, used for every
page that needs OCR within that conversion, then released when the
process exits. This matches the existing isolation pattern for lok
crashes — a Tesseract panic kills one request, not the server.

The subprocess flag contract gains `-ocr <auto|always|never>` and
`-ocr-lang <string>`; both are forwarded by `SubprocessConverter`.
`SubprocessConverter`'s "engine availability" check runs in the
parent at startup (probe for `tesseract` binary or `tessdata` dir,
configurable) so requests that ask for OCR can be rejected with 503
before forking a doomed child.

## New package: `internal/ocr`

The cgo dependency on `gosseract` is isolated to this package, the
same way `*lok.Office` is isolated to `internal/worker/lok_adapter*.go`.

```go
package ocr

type Engine interface {
    // Recognize returns plain UTF-8 text for the given PNG image.
    // langs is a Tesseract language string ("eng", "eng+jpn", …) or
    // "" for OSD-driven script detection.
    Recognize(ctx context.Context, image []byte, langs string) (string, error)
    Close() error
}

type Config struct {
    TessdataPath string   // resolved at startup
    Languages    []string // verified installed; e.g.
                          // ["eng","jpn","chi_sim","chi_tra","osd"]
    DPI          float64  // 300
}

// Constructor is `New` so callers don't bake the engine choice in.
// The implementation today is gosseract-backed; future
// implementations (subprocess, cloud) satisfy the same Engine.
func New(cfg Config) (Engine, error)
```

Tests in `internal/worker` and `internal/server` use a fake `Engine`,
identical in spirit to today's `lok` fakes. Real `gosseract`
exercises live behind `-tags=integration`.

## Worker integration

### Types

```go
// internal/worker/types.go
type OCRMode int
const (
    OCRAuto OCRMode = iota
    OCRAlways
    OCRNever
)

type Job struct {
    // ... existing fields ...
    OCRMode OCRMode // markdown only
    OCRLang string  // markdown only; allowlisted upstream
}

type Config struct {
    // ... existing fields ...
    OCR ocr.Engine // optional; nil disables the feature
}
```

In the subprocess (`cmd/bi/convert.go`), `ocr.New(...)` is called
once and assigned to `worker.Config.OCR` before `worker.New`. In the
parent (`cmd/bi/serve.go`), no OCR engine is constructed — the
parent only checks at startup that the OCR install exists (file
probe) and stores that boolean on `SubprocessConverter`.

### Pipeline (`run_markdown_pdf.go`)

The current `extractPDFText` returns the whole PDF in one shot. The
restructured pipeline operates page-by-page so it can decide per-page
whether to OCR:

1. Open the PDF (existing `ledongthuc/pdf` reader).
2. Walk pages 1..N. For each page:
   1. Extract text rows.
   2. If `OCRMode==OCRNever` or (`OCRMode==OCRAuto` and the extracted
      text is at/above the threshold) → use the extracted text.
   3. Otherwise, if engine is non-nil, render via
      `lokDoc.RenderPagePNG(pageIndex, cfg.OCRDPI)`.
   4. Resolve the language for this page:
      - If `OCRLang` is a concrete value (`eng`, `eng+jpn`, …): use it.
      - If `OCRLang == "all"`: use `eng+jpn+chi_sim+chi_tra`.
      - If `OCRLang == "auto"`: call `Recognize(png, "")` for OSD,
        map detected script (Latin → `eng`, Japanese → `jpn`,
        HanS → `chi_sim`, HanT → `chi_tra`), then call `Recognize`
        again with that language. Two recognize calls per `auto` page.
   5. Emit a leading `<!-- ocr: <lang> page=N -->` comment line, then
      the recognized text.
3. Join pages with the existing page-break separator (`\n\n---\n\n`).

### Output shape

```
<text-extracted page 1>

---

<!-- ocr: jpn page=2 -->
<recognized text page 2>

---

<text-extracted page 3>
```

OCR'd pages are emitted as plain paragraphs separated by blank lines.
No structural inference (no headings, no lists) — Tesseract's
structural cues are unreliable enough on CJK that promoting them to
markdown structure creates more bugs than it prevents.

## HTTP layer

`internal/server/handler_markdown.go`:

- Parse `ocr` and `ocr_lang` query params, allowlist them, default
  to `auto` / `auto`.
- Populate `Job.OCRMode` and `Job.OCRLang`.
- Validation errors → 400 via existing `problem.go`.
- If the parent's startup probe found no Tesseract install and the
  request asks for `ocr=always` or `ocr=auto`, return 503 with
  `code=ocr_unavailable` *before* spawning the child. `ocr=never`
  proceeds normally regardless of OCR availability.

## Configuration

`internal/config.Config` gains:

| Key                 | Env / flag           | Default                                     |
|---------------------|----------------------|---------------------------------------------|
| `OCREnabled`        | `BI_OCR_ENABLED`     | `true`                                      |
| `OCRTessdataPath`   | `BI_OCR_TESSDATA`    | `$TESSDATA_PREFIX` else `/usr/share/tesseract-ocr/5/tessdata` |
| `OCRTextThreshold`  | `BI_OCR_THRESHOLD`   | `16`                                        |
| `OCRDPI`            | `BI_OCR_DPI`         | `300`                                       |

At startup, if `OCREnabled`:

1. Resolve tessdata path.
2. Verify presence of `eng.traineddata`, `jpn.traineddata`,
   `chi_sim.traineddata`, `chi_tra.traineddata`, `osd.traineddata`.
3. Construct `ocr.Engine` and inject into `worker.Config.OCR`.

A missing language pack is a **startup error**, not a request-time
error. Operators see the failure on boot, not on traffic.

## Docker

Runtime stage of `Dockerfile` adds:

```dockerfile
RUN apt-get update && apt-get install -y --no-install-recommends \
      tesseract-ocr \
      tesseract-ocr-eng \
      tesseract-ocr-jpn \
      tesseract-ocr-chi-sim \
      tesseract-ocr-chi-tra \
      tesseract-ocr-osd \
      libtesseract-dev libleptonica-dev \
    && rm -rf /var/lib/apt/lists/*
ENV TESSDATA_PREFIX=/usr/share/tesseract-ocr/5/tessdata
```

Build stage links against `libtesseract` / `libleptonica` (cgo).
`Dockerfile.test` mirrors. Healthcheck fixture gains a tiny scanned
PDF round-trip so a broken `tessdata` install fails the
healthcheck — consistent with the existing "no shallow `/healthz`"
rule.

## Error semantics

| Condition                                       | Behaviour                                                           |
|------------------------------------------------|---------------------------------------------------------------------|
| Per-page recognize error or timeout            | Emit `<!-- ocr-error: <reason> page=N -->` for that page; continue. |
| All OCR'd pages fail                           | 502 with `code=ocr_failed`.                                         |
| Encrypted PDF                                  | Unchanged: 422. OCR never runs on docs we can't open.               |
| Invalid `ocr` / `ocr_lang`                     | 400.                                                                |
| `ocr=always` or `ocr=auto` with engine nil     | 503 `code=ocr_unavailable`.                                         |
| `ocr=never` with engine nil                    | Request proceeds normally; OCR is simply not invoked.               |
| Panic inside cgo OCR                           | Caught at worker boundary (existing pattern); worker is replaced.   |

## Testing

- `internal/ocr` (unit, plain Go): table tests against a fake
  Tesseract via the `Engine` interface.
- `internal/ocr` (`-tags=integration`): per-script fixture images
  (`testdata/ocr/eng.png`, `jpn.png`, `chi_sim.png`, `chi_tra.png`)
  exercising the real gosseract path. Skipped when tessdata is
  unavailable.
- `internal/worker` (unit): markdown-pdf tests parameterised with a
  fake `Engine`. Cover:
  1. Auto threshold boundary (one short page → OCR'd, one long page → not).
  2. `always` forces OCR over a digital page.
  3. `never` skips OCR even on empty pages.
  4. Per-page recognize error becomes a `<!-- ocr-error: ... -->` comment.
  5. `ocr_lang` allowlist (concrete, `+`-joined, `auto`, `all`).
  6. `auto` lang triggers OSD then a second recognize with the
     mapped language.
  7. All-pages-fail returns the worker error that maps to 502.
- `internal/server`: handler tests for query-param validation,
  default values, and the 503 path when engine is nil.
- Integration (`-tags=integration`): four-page scanned PDF fixture
  (one page per script) round-trips through
  `/markdown?ocr=auto&ocr_lang=auto` and yields non-empty text per
  page.
- Coverage gate (≥90%) holds. cgo edges (the gosseract adapter) live
  behind `internal/ocr` interfaces and stay outside coverage the
  same way `lok_adapter.go` does today.

## Open follow-ups (later milestones, not v1)

- Subprocess `Engine` implementation for crash isolation.
- Cloud `Engine` (Google Vision, AWS Textract) behind the same
  interface.
- OCR for office docs that LO has already flattened to images.
- Confidence-driven post-processing (e.g. fall back to OCR even when
  extraction succeeded but produced low-confidence garbage).
