# bi — HTTP API Design

**Status:** Approved 2026-04-28. Ready for implementation planning.
**Scope:** v1 of the `bi` service — a dockerized HTTP wrapper around
`github.com/julianshen/golibreofficekit/lok` that converts office documents to
PDF, PNG (per-page or thumbnail), and Markdown.

This document is the single source of truth for the API contract, package
boundaries, error model, and testing strategy. Implementation diverges from
this spec only via a follow-up edit to this file.

## Goals

- Convert `.docx` / `.xlsx` / `.pptx` / `.odt` / `.ods` / `.odp` (and any
  other format LibreOffice can open) to PDF, PNG, and Markdown over HTTP.
- Run as a single container; one LibreOffice install, one Go binary, no
  external state.
- Honour the `golibreofficekit` upstream constraints (one `lok_init` per
  process, serialise calls per `*lok.Office`).
- Keep the cgo surface tiny and testable: handlers and the Markdown
  pipeline must be unit-testable without a LibreOffice install.

## Non-goals (v1)

- Asynchronous job queues, persistent job state, or webhook callbacks.
- Authentication beyond a single static bearer token.
- Multipart uploads. Raw body only.
- Editing operations. The service is read-only conversion.
- Windows support (matches upstream).

## Architecture

```
                       ┌──────────────────────────────────────────┐
                       │              cmd/bi (binary)             │
                       └──────────────────────────────────────────┘
                                          │
              ┌───────────────────────────┼─────────────────────────────┐
              ▼                           ▼                             ▼
   ┌────────────────────┐    ┌──────────────────────────┐    ┌──────────────────────┐
   │ internal/config    │    │ internal/server          │    │ internal/worker      │
   │  (pure Go)         │    │  (pure Go; no lok)       │    │  (only cgo importer) │
   │                    │    │                          │    │                      │
   │  - flags + env     │    │  - chi router            │    │  - owns *lok.Office  │
   │  - LOK path        │    │  - 4 conversion handlers │    │  - bounded job queue │
   │  - limits/timeouts │    │  - /healthz /readyz      │    │  - per-format Run()  │
   │  - auth token      │    │  - /metrics              │    │  - error classifier  │
   │  - OTel envs       │    │  - middleware chain      │    │  - lokOffice         │
   │                    │    │                          │    │    interface for     │
   │                    │    │                          │    │    test injection    │
   └────────────────────┘    └─────────────┬────────────┘    └──────────┬───────────┘
                                           │                            │
                                           └──── Converter ◄────────────┘
                                                interface

                       ┌──────────────────────────────────────────┐
                       │ internal/mdconv (pure Go)                │
                       │  HTML → Markdown pipeline used by the    │
                       │  Markdown branch of the worker.          │
                       └──────────────────────────────────────────┘
```

### Invariants enforced by the import graph

1. Only `internal/worker` imports `github.com/julianshen/golibreofficekit/lok`.
2. `internal/server` depends only on the `worker.Converter` interface, so
   handlers can be unit-tested with a fake worker that never touches LO.
3. `internal/mdconv` has no cgo and no `lok` import; it operates purely on
   HTML bytes produced by the worker.

### Concurrency model

- One process, one `*lok.Office` (LOK enforces this via `ErrAlreadyInitialised`).
- A bounded job queue (`Workers × 2` deep) provides backpressure. Past the
  cap, callers receive `429 Too Many Requests` with `Retry-After: 1`.
- `lok` itself serialises every call through a per-Office mutex. Our queue
  exists for memory bounding (one large render at a time, not 50 queued
  behind it eating heap) and observable backpressure, not for thread safety.
- A per-conversion timeout (default 120 s) bounds worker time. Because LOK
  has no cancellation API, a render started just before the deadline runs
  to completion server-side; only the result is discarded. The 120 s default
  is deliberately shorter than typical proxy idle timeouts (300 s+).

## HTTP API

All routes are versioned under `/v1/`. Request bodies are raw document bytes
(no multipart). Per-route response content-types are fixed.

| Route                                  | Method | Request body         | Response body       | Notes |
|----------------------------------------|--------|----------------------|---------------------|-------|
| `/v1/convert/pdf`                      | POST   | document bytes       | `application/pdf`   | Whole-document export. |
| `/v1/convert/png?page=N&dpi=F`         | POST   | document bytes       | `image/png`         | Single page; `X-Total-Pages: M` always set. `page` 0-based, defaults to 0. `dpi` defaults to 1.0, range [0.1, 4.0]. |
| `/v1/convert/markdown?images=embed\|drop` | POST | document bytes      | `text/markdown`     | Pipeline: LO → HTML → `mdconv`. `images` defaults to `embed`. |
| `/v1/thumbnail?dpi=F`                  | POST   | document bytes       | `image/png`         | Equivalent to `/v1/convert/png?page=0&dpi=0.5` by default. `X-Total-Pages` set. |
| `/healthz`                             | GET    | —                    | `text/plain`        | Liveness. 200 iff process responds. |
| `/readyz`                              | GET    | —                    | `text/plain`        | Readiness. Runs a real conversion of the embedded fixture; 200 on success, 503 otherwise. Last result cached for `BI_READYZ_CACHE_TTL` (default 5 s). |
| `/metrics`                             | GET    | —                    | Prometheus exposition | Always reachable; not gated by auth. |

### Request constraints

- `Content-Type` header on conversion routes must be present and non-empty
  but is not deeply inspected — LO sniffs the body. Empty `Content-Type` →
  `415 Unsupported Media Type`.
- Body capped at `BI_MAX_UPLOAD_BYTES` (default 100 MiB) on the four
  conversion routes (`/v1/convert/*`, `/v1/thumbnail`). Trips
  `http.MaxBytesReader` → `413 Payload Too Large`. Health and metrics
  routes have no body cap because they take no body.
- Optional `X-Bi-Password` header for encrypted documents. The access-log
  middleware redacts this header before logging. Redaction has its own
  unit test.

### Response headers

- `X-Total-Pages: N` set on every successful PNG and PDF response.
- `X-Bi-Request-Id` set on every response (request-id middleware).
- Standard tracing headers (`traceparent`, etc.) emitted by `otelhttp`.

### Authentication

- If `BI_API_TOKEN` is set, requests must carry `Authorization: Bearer <token>`;
  missing or wrong token returns 401. If unset, auth is disabled.
- `/metrics` and `/healthz` are not auth-gated. `/readyz` is not auth-gated.

## Error model

Error responses use `application/problem+json` (RFC 7807). The `type` URI is
stable and machine-parseable.

```json
{
  "type": "https://bi/errors/password-required",
  "title": "Password required",
  "status": 422,
  "detail": "The document is encrypted; supply X-Bi-Password.",
  "instance": "/v1/convert/pdf",
  "request_id": "01JX..."
}
```

### Status code mapping

| Condition                                               | Status | `type` slug                  |
|---------------------------------------------------------|--------|------------------------------|
| Bad query parameter                                     | 400    | `bad-request`                |
| Missing/empty `Content-Type` on conversion routes       | 415    | `unsupported-media-type`     |
| Body exceeds `BI_MAX_UPLOAD_BYTES`                      | 413    | `payload-too-large`          |
| Encrypted document, no password supplied                | 422    | `password-required`          |
| Encrypted document, wrong password                      | 422    | `password-wrong`             |
| LO cannot parse document                                | 422    | `unsupported-document`       |
| Markdown pipeline cannot produce sensible output        | 500    | `markdown-pipeline`          |
| Worker pool closed (server shutting down)               | 503    | `shutting-down`              |
| `lok.ErrUnsupported` (stripped LibreOffice build)       | 501    | `lok-unsupported`            |
| Worker queue full                                       | 429    | `queue-full` (`Retry-After: 1`) |
| Per-conversion timeout exceeded                         | 504    | `timeout`                    |
| Client disconnected during conversion                   | 499*   | (logged; no body sent — status used in logs/metrics only) |
| Anything else                                           | 500    | `internal`                   |

The worker has a single `classify(err) → sentinel` function. It checks
`errors.Is(err, lok.ErrUnsupported)` first, then string-sniffs `LOKError`
text for password keywords, then falls through to `ErrUnsupportedFormat`.
String matching on LOK error text is fragile but unavoidable until upstream
exposes typed sentinels for password and parse errors; the matcher is
isolated to one file so the upgrade path is one diff.

## Worker package contract

```go
package worker

type Format int
const (
    FormatPDF Format = iota
    FormatPNG
    FormatMarkdown
)

type Job struct {
    InPath        string  // temp file already on disk; worker does not own it
    Format        Format
    Page          int     // 0-based; PNG only
    DPI           float64 // PNG only
    Password      string  // empty if not encrypted
    MarkdownImages MarkdownImageMode // Embed | Drop
}

type Result struct {
    OutPath    string  // worker-owned temp file; caller must os.Remove after streaming
    TotalPages int     // populated for PNG and PDF; 0 for Markdown
    MIME       string  // "application/pdf" / "image/png" / "text/markdown"
}

type Converter interface {
    Run(ctx context.Context, job Job) (Result, error)
}

type Pool struct { /* … */ }

// Config is worker-package-local (distinct from internal/config). The server
// constructs it from the validated runtime config.
type Config struct {
    LOKPath         string
    Workers         int
    QueueDepth      int
    ConvertTimeout  time.Duration
}

func New(cfg Config) (*Pool, error)
func (p *Pool) Run(ctx context.Context, job Job) (Result, error)
func (p *Pool) Close() error
```

### Sentinels

```go
var (
    ErrQueueFull          = errors.New("worker: queue full")
    ErrPasswordRequired   = errors.New("worker: password required")
    ErrWrongPassword      = errors.New("worker: wrong password")
    ErrUnsupportedFormat  = errors.New("worker: unsupported document")
    ErrLOKUnsupported     = errors.New("worker: LOK build lacks required slot")
    ErrMarkdownConversion = errors.New("worker: markdown pipeline failed")
)
```

### Test injection seam

```go
// Internal to internal/worker.
type lokOffice interface {
    Load(path string, opts ...lok.LoadOption) (lokDocument, error)
    Close() error
}
type lokDocument interface {
    SaveAs(path, filter, options string) error
    InitializeForRendering(arg string) error
    RenderPagePNG(page int, dpi float64) ([]byte, error)
    GetParts() int
    Close() error
}
```

Production wires `*lok.Office` and `*lok.Document` through trivial adapters in
`worker/lok_adapter.go` (the **only** file in the repository that imports
`lok`). Tests inject a `fakeOffice` that records calls and returns scripted
errors.

## Markdown pipeline (`internal/mdconv`)

Pipeline:

1. Worker calls `doc.SaveAs(tmpHTML, "html", "")` to get HTML from
   LibreOffice.
2. Worker calls `mdconv.Convert(htmlBytes, mdconv.Options{Images: Embed | Drop})`
   to produce Markdown bytes.
3. Worker writes Markdown to a temp file and returns it as the `Result`.

`mdconv` builds on `github.com/JohannesKaufmann/html-to-markdown/v2` with
custom rules for:

- Tables — force GFM pipe syntax even when LO emits nested table HTML.
- Images — `Embed` inlines as `data:` URIs; `Drop` strips entirely.
- LO style noise — strip `<font>` tags and inline `style="..."` attributes.
- Heading hierarchy — normalise so the first heading is `#`, regardless of
  what LO emitted.
- Footnotes — emit GFM footnote syntax.

`mdconv` is independently unit-tested from canned HTML fixtures committed
under `internal/mdconv/testdata/`. It never imports `lok` and never invokes
LibreOffice. This is the single most behaviourally rich part of the
Markdown route, and we want it covered without an integration tag.

### Markdown caveats (documented behaviour)

- Multi-column page layouts collapse to single-column Markdown.
- Page headers/footers are dropped.
- Embedded fonts are not preserved (MD has no concept of font).
- Spreadsheets convert sheet-by-sheet, separated by `---`; rows beyond
  10,000 per sheet are truncated with a `<!-- truncated -->` marker.

## Configuration

All config read from environment with `BI_` prefix; CLI flags on
`bi serve` override env. Defaults given.

| Var                       | Default                          | Notes |
|---------------------------|----------------------------------|-------|
| `BI_LISTEN_ADDR`          | `:8080`                          | |
| `BI_API_TOKEN`            | unset                            | If set, requires `Authorization: Bearer ...`. |
| `BI_WORKERS`              | `min(NumCPU, 4)`                 | |
| `BI_QUEUE_DEPTH`          | `2 × workers`                    | |
| `BI_MAX_UPLOAD_BYTES`     | `104857600` (100 MiB)            | |
| `BI_CONVERT_TIMEOUT`      | `120s`                           | |
| `BI_READYZ_CACHE_TTL`     | `5s`                             | |
| `LOK_PATH`                | platform default                 | Required if no platform default exists. |
| `OTEL_*`                  | passthrough                      | Standard OTel SDK env. |

`cmd/bi` ships two subcommands:

- `bi serve` — runs the HTTP server. Default if no subcommand given.
- `bi healthcheck` — tiny client that exits 0 iff `GET /readyz` returns 200,
  used by the Dockerfile `HEALTHCHECK`. Avoids adding `curl` to the runtime
  image.

## Observability

### Logs

Structured JSON to stdout, one line per request:

```json
{
  "ts": "2026-04-28T13:14:15.123Z",
  "level": "info",
  "msg": "request",
  "request_id": "01JX...",
  "trace_id": "...",
  "method": "POST",
  "path": "/v1/convert/png",
  "status": 200,
  "latency_ms": 842,
  "in_bytes": 4194304,
  "out_bytes": 318274,
  "format": "png",
  "page": 2,
  "total_pages": 50,
  "queue_wait_ms": 12,
  "convert_ms": 829
}
```

`X-Bi-Password` is redacted by the access-log middleware before any header
capture. The redaction has a unit test.

### Metrics (Prometheus, `/metrics`)

- `bi_requests_total{format,status}` — counter
- `bi_conversion_duration_seconds{format}` — histogram (queue wait excluded)
- `bi_queue_wait_seconds{format}` — histogram
- `bi_queue_depth` — gauge
- `bi_worker_busy` — gauge (number of workers currently inside `lok`)
- `bi_lok_errors_total{kind}` — counter, partitioned by sentinel name

### Tracing (OpenTelemetry)

Configured via standard OTel env (`OTEL_EXPORTER_OTLP_ENDPOINT`,
`OTEL_SERVICE_NAME`, etc.). Spans:

- `http.server.request` (root, from `otelhttp` middleware)
- `convert.<format>` (handler-level)
- `queue.wait`
- `lok.load`
- `lok.save_as` / `lok.render_png`
- `mdconv.convert` (Markdown route only)
- `response.write`

No tracing-specific config knobs in `bi` itself; rely on OTel SDK defaults.

## Docker

Multi-stage `Dockerfile` (already scaffolded):

- Stage 1: `golang:1.25-bookworm` builds with `CGO_ENABLED=1`.
- Stage 2: `debian:bookworm-slim` + `libreoffice-core libreoffice-writer
  libreoffice-calc libreoffice-impress libreoffice-draw ca-certificates
  fonts-liberation fonts-dejavu-core`.
- `ENV LOK_PATH=/usr/lib/libreoffice/program`.
- Runs as non-root user `bi` (uid 10001) with a writable `$HOME` (LO writes
  user-profile directories on first call).
- `HEALTHCHECK` calls `bi healthcheck`, which probes `/readyz`.
- No `scratch` / `distroless`. LO needs the full install tree at runtime.

## Testing strategy

| Package                                  | Test type | LO required | Build tag       | Coverage target |
|------------------------------------------|-----------|-------------|-----------------|-----------------|
| `internal/config`                        | unit      | no          | none            | 100% (current)  |
| `internal/mdconv`                        | unit (table-driven on canned HTML) | no | none | ≥95% |
| `internal/server`                        | unit (with `fakeConverter`) | no | none | ≥90% |
| `internal/worker` (Go logic)             | unit (with `fakeOffice`) | no | none | ≥85% |
| `internal/worker` (real LO smoke)        | integration | **yes** | `//go:build integration` | excluded from gate |
| `cmd/bi serve`                           | smoke (boots, `/healthz` 200) | no | none | ≥80% |
| `cmd/bi healthcheck`                     | unit (against fake server) | no | none | ≥90% |

Coverage gate (`make cover-gate`): **per-package self-coverage**, not a
single merged total. `config`, `mdconv`, `server` clear ≥90%; `worker` clears
≥80% — the lower bar acknowledges six filesystem-error branches in the
run_*.go conversion paths (`os.CreateTemp` / `Write` / `Close` failures)
that need OS-level injection to test, plus `Pool.New`'s success branch
which requires a real LO install. Real LO coverage from `make
test-integration` brings the integrated number well above 90%. The
`integration` tag is excluded from the unit-test coverage profile.

The merged-profile approach (`-coverpkg=./internal/...`) was tried and
abandoned: Go writes separate profiles per test binary and the merge
attributes coverage differently than per-binary self-coverage reports,
so the gate measurement was unstable across cold/warm cache.

### Fixtures (`testdata/`)

- `health.docx` — 1-page Hello-World; used by `/readyz` and the integration
  smoke test.
- `simple.docx`, `simple.xlsx`, `simple.pptx`, `simple.odt` — basic
  conversion paths.
- `encrypted.docx` (password `test123`) — exercises password sentinels.
- `corrupt.docx` (truncated zip) — exercises `ErrUnsupportedFormat`.
- `internal/mdconv/testdata/*.html` + `*.md` — golden-file fixtures for the
  HTML→MD pipeline.

All fixtures are vendored under git, each ≤10 KB.

### TDD discipline

Per repository CLAUDE.md rule 3 (strict TDD):

- Each task in the implementation plan owns both its failing test and its
  implementation. Plans MUST NOT split into "implement A, B, C" / "test A,
  B, C" phases.
- Each commit lands the test for the behaviour it introduces. No
  "implement X, tests in next commit" commits.

## Open questions / explicit deferrals

- **Subprocess-per-conversion isolation.** v1 uses a goroutine pool sharing
  one `*lok.Office`. If a wedged or crashing LO becomes a real production
  signal, the `Converter` interface lets a subprocess implementation drop
  in without API changes.
- **Multipart support.** Not in v1. Adding it later is non-breaking
  (sniff `Content-Type`).
- **Async job mode.** Explicitly out of scope for v1. Adding `?async=true`
  later is non-breaking.

## Acceptance criteria

The implementation is complete when:

1. All 7 routes return the documented response for the documented inputs.
2. All status codes in the error mapping table fire on a representative
   negative test.
3. `make cover-gate` passes locally and in CI.
4. `make test-integration` passes in a container with LibreOffice
   installed against every fixture document.
5. `docker build -t bi:dev . && docker run -p 8080:8080 bi:dev` and a
   `curl` round-trip of `simple.docx` → PDF / PNG / Markdown succeeds.
6. The `Dockerfile` `HEALTHCHECK` reports healthy within `start-period`.
