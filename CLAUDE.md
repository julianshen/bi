# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

`bi` is a **dockerized HTTP service** that converts office documents (Writer / Calc /
Impress / Draw formats — `.docx`, `.xlsx`, `.pptx`, `.odt`, …) to:

- **PDF** (full-document export)
- **PNG** (per-page render, plus thumbnails)
- **Markdown** (text export)

It is implemented in **Go** on top of
[`github.com/julianshen/golibreofficekit`](https://github.com/julianshen/golibreofficekit)
(`lok` package), which is a cgo binding for LibreOfficeKit. The repository is currently
empty; the rules below are load-bearing for the first commits.

## Non-negotiable workflow

These rules are inherited from the upstream `golibreofficekit` project and apply here
verbatim. Do not relax them without explicit instruction.

1. **Never commit to `main`.** Branch (`feat/<slug>`, `fix/<slug>`, `chore/<slug>`)
   before editing.
2. **Plan first, then implement.** Use `superpowers:writing-plans` for non-trivial
   work. Each PR should be a small, independently reviewable chunk.
3. **Strict TDD.** Red → green → refactor. Each commit lands the test for the
   behaviour it introduces — never an "implement X, tests later" commit, and never
   a plan structured as "implement A, B, C" then "add tests for A, B, C".
4. **Brainstorm before building.** Invoke `superpowers:brainstorming` before
   `EnterPlanMode` for any new feature or API surface (a new endpoint, a new
   conversion mode, a new CLI flag, etc.).
5. **Verify before claiming done.** Use `superpowers:verification-before-completion`:
   run the exact build/test commands and quote the output before saying something
   passes.
6. **Test coverage stays above 90%.** Measure with
   `go test -covermode=atomic -coverprofile=coverage.out ./...` and report
   `go tool cover -func=coverage.out | tail -n 1`. cgo trampolines that cannot be
   unit-tested live in the upstream `lok` package — this repo's code should be
   pure Go and fully covered.
7. **No shortcuts.** No disabled checks, relaxed lint rules, lowered thresholds,
   `// nolint`, skipped build tags, `|| true`, or `--no-verify`. If something is
   hard, understand it.
8. **Don't skip failing tests.** No `t.Skip`, no commented assertions, no
   `_test_disabled` files, no never-set build tags. Fix the code or fix the test.
9. **Complete implementation.** Trace root causes; do not work around crashes by
   removing the call that triggers them, do not silently shrink scope, do not
   leave `// TODO` stubs in shipped code.
10. **Ask before cutting any feature.** If a planned endpoint, option, format, or
    behaviour cannot be implemented as agreed, pause and ask — even removing a
    case from an integration smoke test counts.

## Architecture

### Process model

LibreOfficeKit is **not free-threaded** and **only one `lok_init` per process** is
permitted. The service architecture must respect both:

- The HTTP handler does **not** call `lok` directly. Conversions are dispatched to
  a **worker pool** that owns the singleton `*lok.Office`.
- Concurrency is bounded by the worker count, not the number of in-flight HTTP
  requests. Excess requests queue or are rejected with 429 — never serialized
  silently behind a single mutex with no backpressure.
- A worker that panics or sees LOK state corruption (e.g. failed `Load` after a
  crash on a prior document) must be replaceable. Plan for either (a) restarting
  the whole process under a supervisor, or (b) running each conversion in a
  short-lived child process. Document the choice in the package doc.

### Layers

```
HTTP handler  ──►  Job queue  ──►  Worker (owns *lok.Office)  ──►  golibreofficekit/lok
     │                                       │
     └── multipart upload                    └── temp file lifecycle, output buffer
```

Keep the HTTP layer free of cgo: handlers should accept the upload, write it to
a temp file, push a job, and stream the result back. All `import "C"` lives
behind the `lok` import only.

### Conversion semantics (from upstream `lok`)

- **PDF / Markdown** → `doc.SaveAs(path, filter, options)` with filter strings
  `"pdf"` and `"md"` (or whatever the upstream package documents at the time —
  verify against `pkg.go.dev/github.com/julianshen/golibreofficekit/lok`).
- **PNG / thumbnail** → `doc.InitializeForRendering("")` then
  `doc.RenderPagePNG(pageIndex, dpiScale)`. Thumbnails are just a low DPI scale
  of page 0; do not introduce a parallel rendering path.
- **Errors** → upstream surfaces real LibreOffice error strings (password
  required, filter rejected, etc.) and `ErrUnsupported` for stripped LO builds.
  Map these to meaningful HTTP status codes (4xx for bad input, 5xx for LOK
  failures), do not collapse them all into 500.

### LibreOffice install path

The service needs to know where LibreOffice's `program/` directory is. Follow
the upstream contract:

1. Explicit config (CLI flag / env var on the service binary).
2. `$LOK_PATH`.
3. Platform-default candidates.

In the Docker image this should be set explicitly via `ENV LOK_PATH=...` so the
service does not probe at startup.

## Docker

The deliverable is a container, so the Dockerfile is part of the product, not a
side concern.

- Base image must include LibreOffice. Prefer Debian/Ubuntu (`/usr/lib/libreoffice/program`)
  for smaller `apt`-installed footprints; the `libreoffice-core` + per-app split
  packages (`-writer`, `-calc`, `-impress`, `-draw`) avoid pulling the GUI.
- Multi-stage build: stage 1 compiles the Go binary with cgo enabled against
  the same glibc as the runtime image; stage 2 is the runtime with LibreOffice
  + the binary. **Do not** use `scratch` or `distroless` for the runtime — LOK
  needs the full LibreOffice install tree at runtime.
- Set `ENV LOK_PATH=/usr/lib/libreoffice/program` (or whatever matches the base
  image) so the service binary picks it up without flags.
- Healthcheck should hit a real conversion endpoint with a tiny fixture, not
  just a `/healthz` that returns 200 — a broken LOK install is the most
  likely production failure and a TCP-level check will not catch it.

## Commands

Project scaffolding is not in place yet. Once `go.mod` exists the expected
commands are:

```bash
go build ./...
go vet ./...
go test ./...                                  # unit tests (no LOK required)
go test -tags=integration ./...                # tests that hit real LibreOffice
go test -run TestName ./path/to/pkg            # single test
go test -race ./...                            # strongly recommended

# Coverage gate
go test -covermode=atomic -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | tail -n 1

# Container
docker build -t bi:dev .
docker run --rm -p 8080:8080 bi:dev
```

Tests that actually invoke LibreOffice live behind a build tag and skip when
`LOK_PATH` is unset, so `go test ./...` stays green on CI runners without
LibreOffice installed. Commit small fixture documents (`testdata/hello.docx`,
…) rather than generating them at test time.

Add `gofmt -s -w .` (or `goimports`) to the pre-commit loop. If a linter is
adopted, prefer `golangci-lint run`.

## Style

Follow *Effective Go* and the Go Proverbs:

- Errors are values — return `error`, never panic across a request boundary.
- Accept interfaces, return concrete types.
- `io.Reader` / `io.Writer` at package edges; stream uploads/downloads, do not
  buffer entire documents in memory unless the upstream API forces it.
- `defer` cleanup (temp files, `doc.Close()`, `office.Close()`) immediately
  after acquisition.
- Keep cgo isolated: only the worker package imports `lok`; HTTP, queue, and
  config layers stay pure Go.

## References

- Upstream binding: <https://github.com/julianshen/golibreofficekit>
- `lok` godoc: <https://pkg.go.dev/github.com/julianshen/golibreofficekit/lok>
- LibreOfficeKit overview: <https://wiki.documentfoundation.org/Development/LibreOfficeKit>
