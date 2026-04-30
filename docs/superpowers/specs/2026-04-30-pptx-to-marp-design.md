# pptx/odp → Marp Markdown — Design

**Status:** Approved 2026-04-30. Ready for implementation planning.
**Scope:** Auto-emit [Marp](https://marpit.marp.app/)-style markdown when the
caller posts a presentation file to `POST /v1/convert/markdown`. No new
route, no new query parameters, no breaking changes for existing callers.

## Goals

- A `.pptx` / `.odp` / `.ppt` upload to `/v1/convert/markdown` returns
  markdown with valid Marp slide separators and front-matter.
- A `.docx` / `.odt` / etc. upload behaves exactly as today (flat markdown,
  no separators, no front-matter).
- The decision is automatic — the caller does not pass a flag.

## Non-goals (v1)

- Speaker notes. LO's HTML export of `.pptx` does not reliably surface
  notes; we drop them and document the limitation.
- Theme / paginate / size front-matter knobs. Marp uses defaults if absent.
- Per-slide image extraction or `impress_html_Export` (one-file-per-slide)
  filter. Stick with the existing flat `html` filter and detect slide
  breaks in the HTML.
- A standalone `/v1/convert/marp` route. Additive behaviour on the existing
  route is cleaner.

## Detection

The handler inspects the request `Content-Type`. If it is one of:

| Content-Type | Format |
|---|---|
| `application/vnd.openxmlformats-officedocument.presentationml.presentation` | `.pptx` |
| `application/vnd.oasis.opendocument.presentation` | `.odp` |
| `application/vnd.ms-powerpoint` | `.ppt` |

then the handler sets `worker.Job.MarkdownMarp = true`. Otherwise it stays
`false`. The detection helper lives next to `extensionFromContentType` so
the two have one source of truth for "what counts as a presentation".

## Output shape

For a presentation input the response body is:

```markdown
---
marp: true
---

# Slide 1 title

slide 1 body…

---

# Slide 2 title

slide 2 body…
```

- Front-matter is fixed: `marp: true` only. No theme, no paginate, no size.
  Callers who want those run `marp` post-processing on their side.
- Slide separator is `---` on its own line, with one blank line before and
  after, matching Marpit's documented form.
- Existing markdown rules continue to apply per-slide: heading
  normalisation, image embed/drop, LO style scrubbing.

For a non-presentation input the response body is unchanged from today —
no front-matter, no separators.

## Architecture

The pipeline does not change shape:

```
worker.runMarkdown
  ├─ doc.SaveAs(htmlPath, "html", "")          (LO emits flat HTML)
  ├─ os.ReadFile(htmlPath)                     (read bytes)
  └─ p.md.Convert(htmlBytes, mode, base, marp) (mdconv processes)
        └─ mdconv.ConvertWithBase
              ├─ scrubLONoise
              ├─ defaultConv.ConvertString    (HTML → flat markdown)
              ├─ normaliseHeadings
              ├─ applyImageMode
              └─ applyMarp                    (NEW; only when Marp=true)
```

`applyMarp` runs as a post-processing step on the markdown bytes the rest
of the pipeline already produces. Reusing the existing converter means
table / list / escaping behaviour is inherited untouched.

### Slide-break detection

LO's `html` filter emits `<hr/>` (and variants like
`<hr style="page-break-before:always"/>`) between slides on `.pptx` /
`.odp` exports. After the HTML→markdown conversion these become a markdown
horizontal rule (`---` or `* * *`).

`applyMarp`:

1. Splits the markdown into segments at horizontal-rule boundaries
   (regex `(?m)^\s*(---|\*\s*\*\s*\*)\s*$`).
2. For each segment, trims leading/trailing blank lines.
3. Rejoins with `\n\n---\n\n` between segments.
4. Prepends `---\nmarp: true\n---\n\n`.

Edge cases:
- Zero `<hr>` markers in the HTML (unusual but possible — single-slide
  decks): one segment, output is `front-matter + body`. No internal `---`.
- A document author wrote a literal `---` inside slide content: same
  splitter consumes it as a slide break. Documented as a known limitation;
  vanishingly rare in practice and an unavoidable consequence of treating
  the markdown as the source of truth.

## Components

### New

- `internal/mdconv/rules_marp.go`
  - `applyMarp(md []byte) []byte`
- `internal/mdconv/Options.Marp bool`

### Modified

- `internal/mdconv/convert.go`
  - `ConvertWithBase` calls `applyMarp` if `opts.Marp`
- `internal/worker/types.go`
  - `Job.MarkdownMarp bool` field
- `internal/worker/iface.go`
  - `htmlToMarkdown.Convert` gains a `marp bool` parameter
- `internal/worker/pool.go`
  - `mdAdapter.Convert` forwards `marp` to mdconv
- `internal/worker/run_markdown.go`
  - passes `job.MarkdownMarp` into `p.md.Convert`
- `internal/server/handler_markdown.go`
  - sets `job.MarkdownMarp = isPresentationContentType(ct)`
- `internal/server/handler_pdf.go`
  - new helper `isPresentationContentType(ct string) bool` adjacent to
    `extensionFromContentType` (one source of truth for presentation
    detection)
- `cmd/bi/convert.go`
  - new `-marp` flag forwarded to the pool
- `internal/server/subprocess.go`
  - `SubprocessConverter.Run` adds `-marp` to args when
    `job.MarkdownMarp == true`
- `docs/superpowers/specs/2026-04-28-bi-http-api-design.md`
  - note the auto-Marp behaviour under `/v1/convert/markdown`
- `CLAUDE.md` — no change required; the architectural invariants hold.

## Error model

No new sentinels. The only failure modes are the existing markdown
pipeline failures (`ErrMarkdownConversion`) and existing server problems.
Marp-mode does not introduce new client-visible errors.

## Testing

### `internal/mdconv` (unit, golden-file)

Add four fixture pairs under `internal/mdconv/testdata/`:

- `marp-simple.html` / `marp-simple.md` — 3 slides, headings + paragraphs
- `marp-with-image.html` / `marp-with-image.md` — slide containing an
  image; verifies `applyImageMode` ran first, Marp split second
- `marp-no-slides.html` / `marp-no-slides.md` — input has no `<hr>`
  markers; output is front-matter + single body, no inner `---`
- `marp-empty-slide.html` / `marp-empty-slide.md` — `<hr/><hr/>` adjacent
  in source; an empty segment between is dropped (no `\n\n---\n\n---\n\n`)

Test harness extends `TestConvertGolden` with a `marp` flag column;
fixtures with the `marp-` prefix are processed with `Options.Marp = true`.

### `internal/worker`

Add `TestPoolRunMarkdownPropagatesMarpFlag` to `run_markdown_test.go`:
fakeMD captures the `marp` argument; assert it equals `job.MarkdownMarp`.

### `internal/server`

Add to `handler_markdown_test.go`:

- `TestMarkdownAutoEnablesMarpForPPTX` — POST with pptx Content-Type sets
  `MarkdownMarp` on the job (verified via fakeConverter)
- `TestMarkdownAutoEnablesMarpForODP` — same for odp
- `TestMarkdownDisablesMarpForDOCX` — verifies the existing default

### Smoke test (manual)

Verified by `make docker-test` against the integration tagged tests; no
new fixture needed (the existing `health.docx` exercises the
non-presentation path; a small pptx fixture committed under
`testdata/health.pptx` exercises the presentation path).

## Coverage gates

Per-package thresholds unchanged. The new code (`applyMarp` + handler
helper + flag plumbing) is covered by the tests above, so the gates
should hold without further work. If `mdconv` slips below 90 % the test
fixtures are too sparse — add cases.

## Acceptance criteria

The implementation is complete when:

1. `POST /v1/convert/markdown` with a `.pptx` body returns 200 with
   `Content-Type: text/markdown` and a body that begins with
   `---\nmarp: true\n---\n` and contains `\n\n---\n\n` between slides.
2. `POST /v1/convert/markdown` with a `.docx` body returns 200 with the
   same body shape as today (no front-matter, no `---` separators).
3. `make cover-gate` passes, `make docker-test` passes.
4. The docker smoke test on a committed `testdata/health.pptx` returns
   valid Marp markdown with the expected slide count (front-matter
   present, separator count = slides − 1).
5. Optional: round-trips through `npx @marp-team/marp-cli@latest` without
   parser error. Bonus check, not a blocker — the body-format assertions
   in #1 + #4 are the load-bearing acceptance test.
