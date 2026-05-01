# pptx/odp → Marp Markdown Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When a `.pptx` / `.odp` / `.ppt` is posted to `/v1/convert/markdown`, return Marp-style markdown (front-matter + `---` slide separators) automatically. `.docx` etc. behaviour stays the same. Spec at `docs/superpowers/specs/2026-04-30-pptx-to-marp-design.md`.

**Architecture:** Detection lives in the server handler (Content-Type → `Job.MarkdownMarp`). The flag flows through worker → mdAdapter → mdconv. mdconv runs a new `applyMarp` post-processor after the existing `applyImageMode` step: split markdown at horizontal-rule lines (which LO produced from `<hr/>` slide breaks), stitch with `\n\n---\n\n`, prepend `---\nmarp: true\n---\n\n` front-matter.

**Tech Stack:** Same as the existing service — Go 1.25, mdconv with `html-to-markdown/v2`, server with chi router. No new deps. The plumbing follows the precedent of `MarkdownImages` (drop/embed) which already runs through every layer.

**TDD discipline:** Each task lands its failing test plus minimal implementation in one commit, mirroring the original mdconv tasks. No "implement A, B, C then test A, B, C" splits.

**Branch:** `feat/pptx-to-marp` (already created and rebased on `main`).

---

## File map

```
internal/mdconv/
  options.go                  + Marp bool field
  convert.go                  + applyMarp call site
  rules_marp.go               NEW: applyMarp + slideBreakRE
  convert_test.go             + new golden cases (marp-*)
  testdata/
    marp-simple.html / .md      NEW
    marp-with-image.html / .md  NEW (image fixture x.png reused)
    marp-no-slides.html / .md   NEW
    marp-empty-slide.html / .md NEW

internal/worker/
  types.go                    + Job.MarkdownMarp
  iface.go                    + marp param on htmlToMarkdown.Convert
  pool.go                     mdAdapter forwards marp
  run_markdown.go             passes job.MarkdownMarp through
  run_markdown_test.go        + fakeMD captures marp
  pool_md_test.go             + adapter test for marp pass-through

internal/server/
  handler_markdown.go         sets Job.MarkdownMarp from CT
  handler_pdf.go              + isPresentationContentType helper
  handler_markdown_test.go    + 3 cases: pptx/odp/odg→true, docx→false

cmd/bi/
  convert.go                  + -marp flag

internal/server/
  subprocess.go               forwards -marp arg

testdata/
  health.pptx                 NEW (1-slide minimal pptx, ≤2KB)

internal/worker/
  integration_test.go         + TestRealConversionMarkdownMarp
```

---

## Task 1: Generate `testdata/health.pptx` fixture

**Files:**
- Create: `testdata/health.pptx` (binary, programmatic, ≤2 KB)
- Create: `/tmp/gen-pptx.go` (one-shot helper, deleted after run)

A docker-test integration test needs a real-LO-loadable pptx. Like
`health.docx`, build it programmatically — pandoc isn't on the dev host.

- [ ] **Step 1: Write the generator**

Create `/tmp/gen-pptx.go`:

```go
package main

import (
	"archive/zip"
	"bytes"
	"log"
	"os"
)

func main() {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	files := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/>
<Override PartName="/ppt/slides/slide1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>
<Override PartName="/ppt/slides/slide2.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>
<Override PartName="/ppt/slideLayouts/slideLayout1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml"/>
<Override PartName="/ppt/slideMasters/slideMaster1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideMaster+xml"/>
</Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="ppt/presentation.xml"/>
</Relationships>`,
		"ppt/_rels/presentation.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideMaster" Target="slideMasters/slideMaster1.xml"/>
<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide1.xml"/>
<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide2.xml"/>
</Relationships>`,
		"ppt/presentation.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">
<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst>
<p:sldIdLst><p:sldId id="256" r:id="rId2"/><p:sldId id="257" r:id="rId3"/></p:sldIdLst>
<p:sldSz cx="9144000" cy="6858000"/>
</p:presentation>`,
		"ppt/slides/_rels/slide1.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout1.xml"/>
</Relationships>`,
		"ppt/slides/_rels/slide2.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout1.xml"/>
</Relationships>`,
		"ppt/slides/slide1.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">
<p:cSld><p:spTree>
<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>
<p:grpSpPr/>
<p:sp><p:nvSpPr><p:cNvPr id="2" name="Title"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr/>
<p:txBody><a:bodyPr/><a:lstStyle/>
<a:p><a:r><a:t>Slide One</a:t></a:r></a:p>
</p:txBody></p:sp>
</p:spTree></p:cSld>
</p:sld>`,
		"ppt/slides/slide2.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">
<p:cSld><p:spTree>
<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>
<p:grpSpPr/>
<p:sp><p:nvSpPr><p:cNvPr id="2" name="Title"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr/>
<p:txBody><a:bodyPr/><a:lstStyle/>
<a:p><a:r><a:t>Slide Two</a:t></a:r></a:p>
</p:txBody></p:sp>
</p:spTree></p:cSld>
</p:sld>`,
		"ppt/slideLayouts/_rels/slideLayout1.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideMaster" Target="../slideMasters/slideMaster1.xml"/>
</Relationships>`,
		"ppt/slideLayouts/slideLayout1.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sldLayout xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" type="title" preserve="1">
<p:cSld name="Title Slide"><p:spTree>
<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>
<p:grpSpPr/>
</p:spTree></p:cSld>
</p:sldLayout>`,
		"ppt/slideMasters/_rels/slideMaster1.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout1.xml"/>
</Relationships>`,
		"ppt/slideMasters/slideMaster1.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sldMaster xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">
<p:cSld><p:spTree>
<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>
<p:grpSpPr/>
</p:spTree></p:cSld>
<p:sldLayoutIdLst><p:sldLayoutId id="2147483649" r:id="rId1" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"/></p:sldLayoutIdLst>
</p:sldMaster>`,
	}
	for name, body := range files {
		f, err := z.Create(name)
		if err != nil {
			log.Fatal(err)
		}
		if _, err := f.Write([]byte(body)); err != nil {
			log.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(os.Args[1], buf.Bytes(), 0o644); err != nil {
		log.Fatal(err)
	}
}
```

- [ ] **Step 2: Generate the file**

```bash
cd /tmp && go run gen-pptx.go /Users/julianshen/prj/bi/testdata/health.pptx
ls -la /Users/julianshen/prj/bi/testdata/health.pptx
file /Users/julianshen/prj/bi/testdata/health.pptx
```

Expected: file exists, size ~2 KB, `file` reports "Microsoft PowerPoint" or "Zip archive". Either is fine — LO sniffs content.

- [ ] **Step 3: Smoke-test the fixture loads in LO**

Run inside the existing runtime image (LO 7.4.7):

```bash
docker build -t bi:dev .
docker run --rm -v /Users/julianshen/prj/bi/testdata:/td:ro --entrypoint sh bi:dev -c '
soffice --headless --convert-to pdf /td/health.pptx --outdir /tmp 2>&1
ls -la /tmp/*.pdf
'
```

Expected: a PDF appears. If LO refuses the file, the fixture XML is malformed — fix and retry.

- [ ] **Step 4: Commit**

```bash
rm /tmp/gen-pptx.go
git add testdata/health.pptx
git commit -m "$(cat <<'EOF'
test(fixtures): add 2-slide health.pptx for marp integration tests

Generated programmatically via archive/zip — pandoc isn't on the dev
host. Smallest-possible OOXML pptx that LO 7.4.7 can load and convert.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `mdconv.Options.Marp` + first golden test

**Files:**
- Modify: `internal/mdconv/options.go`
- Create: `internal/mdconv/rules_marp.go`
- Modify: `internal/mdconv/convert.go`
- Modify: `internal/mdconv/convert_test.go`
- Create: `internal/mdconv/testdata/marp-simple.html`
- Create: `internal/mdconv/testdata/marp-simple.md`

- [ ] **Step 1: Write the failing test fixture pair**

`internal/mdconv/testdata/marp-simple.html` (LO-style 3-slide HTML — `<hr/>` between slides is the LO convention):

```html
<html><body>
<h1>Slide One</h1>
<p>First slide body.</p>
<hr/>
<h1>Slide Two</h1>
<p>Second slide body.</p>
<hr/>
<h1>Slide Three</h1>
<p>Third slide body.</p>
</body></html>
```

`internal/mdconv/testdata/marp-simple.md`:

```
---
marp: true
---

# Slide One

First slide body.

---

# Slide Two

Second slide body.

---

# Slide Three

Third slide body.
```

- [ ] **Step 2: Extend convert_test.go to drive marp cases**

Modify `TestConvertGolden` in `internal/mdconv/convert_test.go`. Replace the existing `cases` slice with one that records whether each case opts into Marp:

```go
cases := []struct {
    name string
    marp bool
}{
    {"paragraph", false},
    {"headings", false},
    {"table", false},
    {"image-embed", false},
    {"image-drop", false},
    {"lo-noise", false},
    {"marp-simple", true},
}
```

Update the loop body to pass the flag:

```go
for _, c := range cases {
    t.Run(c.name, func(t *testing.T) {
        html := mustRead(t, filepath.Join("testdata", c.name+".html"))
        wantMD := mustRead(t, filepath.Join("testdata", c.name+".md"))
        opts := mdconv.Options{Images: mdconv.ImagesEmbed, Marp: c.marp}
        if c.name == "image-drop" {
            opts.Images = mdconv.ImagesDrop
        }
        if c.name == "image-embed" || c.name == "image-drop" {
            seedSiblingImage(t, "testdata/x.png", "PNGFAKE")
            t.Cleanup(func() { _ = os.Remove("testdata/x.png") })
        }
        gotMD, err := mdconv.Convert(html, opts)
        if err != nil {
            t.Fatalf("Convert: %v", err)
        }
        if normalise(gotMD) != normalise(wantMD) {
            t.Errorf("output mismatch:\n got: %q\nwant: %q", gotMD, wantMD)
        }
    })
}
```

- [ ] **Step 3: Run, expect failure**

```bash
go test ./internal/mdconv/ -run TestConvertGolden -v
```

Expected: build failure (`undefined: mdconv.Options field Marp`).

- [ ] **Step 4: Add `Marp` field to Options**

Replace `internal/mdconv/options.go` with:

```go
package mdconv

type ImageMode int

const (
    ImagesEmbed ImageMode = iota
    ImagesDrop
)

type Options struct {
    Images ImageMode
    Marp   bool
}
```

- [ ] **Step 5: Implement applyMarp**

Create `internal/mdconv/rules_marp.go`:

```go
package mdconv

import (
    "bytes"
    "regexp"
)

// slideBreakRE matches a markdown horizontal rule on its own line.
// LO emits <hr/> between slides on .pptx/.odp HTML export; html-to-markdown
// renders that as either "---" or "* * *" (or "***"). We accept all three.
var slideBreakRE = regexp.MustCompile(`(?m)^\s*(?:-{3,}|\*{3,}|(?:\*\s*){3,})\s*$`)

// applyMarp wraps a flat markdown body in Marp front-matter and replaces
// horizontal-rule slide breaks with normalised `---` separators (one
// blank line on each side). Empty segments — e.g. consecutive <hr/> in
// the source — are dropped so the output never contains adjacent
// separators.
func applyMarp(md []byte) []byte {
    indices := slideBreakRE.FindAllIndex(md, -1)
    var segments [][]byte
    cursor := 0
    for _, m := range indices {
        seg := bytes.TrimSpace(md[cursor:m[0]])
        if len(seg) > 0 {
            segments = append(segments, seg)
        }
        cursor = m[1]
    }
    tail := bytes.TrimSpace(md[cursor:])
    if len(tail) > 0 {
        segments = append(segments, tail)
    }

    var buf bytes.Buffer
    buf.WriteString("---\nmarp: true\n---\n\n")
    for i, seg := range segments {
        if i > 0 {
            buf.WriteString("\n\n---\n\n")
        }
        buf.Write(seg)
    }
    buf.WriteByte('\n')
    return buf.Bytes()
}
```

- [ ] **Step 6: Wire applyMarp into ConvertWithBase**

Modify `internal/mdconv/convert.go`. Find:

```go
func ConvertWithBase(html []byte, opts Options, base string) ([]byte, error) {
    html = scrubLONoise(html)
    md, err := defaultConv.ConvertString(string(html))
    if err != nil {
        return nil, err
    }
    out := normaliseHeadings([]byte(md))
    out = applyImageMode(out, opts.Images, base)
    return out, nil
}
```

Replace with:

```go
func ConvertWithBase(html []byte, opts Options, base string) ([]byte, error) {
    html = scrubLONoise(html)
    md, err := defaultConv.ConvertString(string(html))
    if err != nil {
        return nil, err
    }
    out := normaliseHeadings([]byte(md))
    out = applyImageMode(out, opts.Images, base)
    if opts.Marp {
        out = applyMarp(out)
    }
    return out, nil
}
```

- [ ] **Step 7: Run, expect pass**

```bash
go test ./internal/mdconv/ -run TestConvertGolden -v
```

Expected: all 7 subtests pass, including `marp-simple`. If `marp-simple` fails on whitespace, run with `-v` and adjust the golden to match the actual output (the `normalise` helper trims trailing-line whitespace, so spacing inside a line is byte-exact).

- [ ] **Step 8: Commit**

```bash
git add internal/mdconv/
git commit -m "$(cat <<'EOF'
feat(mdconv): applyMarp post-processor + Options.Marp + first golden

When opts.Marp is true, emit Marp front-matter and normalise horizontal-
rule slide breaks to `---` separators. Empty segments between consecutive
breaks are dropped. Slide-break detection accepts ---, ***, and * * *
forms because html-to-markdown emits any of them depending on input HTML.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Additional Marp golden cases

**Files:**
- Create: `internal/mdconv/testdata/marp-no-slides.html`
- Create: `internal/mdconv/testdata/marp-no-slides.md`
- Create: `internal/mdconv/testdata/marp-empty-slide.html`
- Create: `internal/mdconv/testdata/marp-empty-slide.md`
- Create: `internal/mdconv/testdata/marp-with-image.html`
- Create: `internal/mdconv/testdata/marp-with-image.md`
- Modify: `internal/mdconv/convert_test.go`

- [ ] **Step 1: Add fixtures**

`marp-no-slides.html` (HTML with no `<hr/>`):

```html
<html><body>
<h1>Only Slide</h1>
<p>Just one slide; no separators in the source.</p>
</body></html>
```

`marp-no-slides.md`:

```
---
marp: true
---

# Only Slide

Just one slide; no separators in the source.
```

`marp-empty-slide.html` (adjacent `<hr/>` markers — common when LO emits a
title-only slide followed by another):

```html
<html><body>
<h1>One</h1>
<hr/>
<hr/>
<h1>Three</h1>
</body></html>
```

`marp-empty-slide.md` (the empty middle segment is dropped):

```
---
marp: true
---

# One

---

# Three
```

`marp-with-image.html` — verifies `applyImageMode` runs first, then Marp
splits:

```html
<html><body>
<h1>Slide With Image</h1>
<img src="x.png" alt="alt"/>
<hr/>
<h1>Plain Slide</h1>
<p>No image here.</p>
</body></html>
```

`marp-with-image.md` (mode = embed; "PNGFAKE" → text/plain MIME because
the bytes don't match a real PNG header — same convention as
`image-embed.md`):

```
---
marp: true
---

# Slide With Image

![alt](data:text/plain; charset=utf-8;base64,UE5HRkFLRQ==)

---

# Plain Slide

No image here.
```

- [ ] **Step 2: Add new cases to convert_test.go**

Extend the `cases` slice:

```go
cases := []struct {
    name string
    marp bool
}{
    {"paragraph", false},
    {"headings", false},
    {"table", false},
    {"image-embed", false},
    {"image-drop", false},
    {"lo-noise", false},
    {"marp-simple", true},
    {"marp-no-slides", true},
    {"marp-empty-slide", true},
    {"marp-with-image", true},
}
```

The `marp-with-image` case needs the same `seedSiblingImage` setup as
`image-embed`. Update the loop body:

```go
if c.name == "image-embed" || c.name == "image-drop" || c.name == "marp-with-image" {
    seedSiblingImage(t, "testdata/x.png", "PNGFAKE")
    t.Cleanup(func() { _ = os.Remove("testdata/x.png") })
}
```

- [ ] **Step 3: Run, all pass**

```bash
go test ./internal/mdconv/ -run TestConvertGolden -v
```

Expected: 10 subtests pass. If `marp-with-image` fails on MIME mismatch
(e.g. LO actually emits a different placeholder), update the golden md
to match what the run prints.

- [ ] **Step 4: Commit**

```bash
git add internal/mdconv/
git commit -m "$(cat <<'EOF'
test(mdconv): cover no-slides, empty-slide, and image+marp combinations

Three more golden pairs round out the applyMarp surface. The image case
proves applyImageMode runs first and applyMarp second, which is the
order ConvertWithBase enforces.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Plumb `marp` through worker → mdAdapter

**Files:**
- Modify: `internal/worker/types.go`
- Modify: `internal/worker/iface.go`
- Modify: `internal/worker/pool.go`
- Modify: `internal/worker/run_markdown.go`
- Modify: `internal/worker/run_markdown_test.go`
- Modify: `internal/worker/pool_md_test.go`
- Modify: `internal/worker/pool_extra_test.go`

- [ ] **Step 1: Write the failing test**

In `internal/worker/run_markdown_test.go`, add inside the existing
`TestPoolRunMarkdownHappyPath` after the `md.images` check and BEFORE
the `md.base` check:

```go
    if md.marp {
        t.Errorf("md.marp = true, want false (default)")
    }
```

Add a new test below the existing `TestPoolRunMarkdownConvertError`:

```go
func TestPoolRunMarkdownPropagatesMarpFlag(t *testing.T) {
    doc := &fakeDocument{parts: 1}
    doc.saveAsHook = func(path, _, _ string) error {
        return os.WriteFile(path, []byte("<p>hi</p>"), 0o600)
    }
    office := &fakeOffice{loadDoc: doc}
    p, _ := newWithOffice(Config{Workers: 1, QueueDepth: 1, ConvertTimeout: time.Second}, office)
    t.Cleanup(func() { _ = p.Close() })

    md := &fakeMD{out: []byte("ok")}
    p.setMarkdown(md)

    in := tmpFile(t, "deck.pptx", []byte("x"))
    _, err := p.Run(context.Background(), Job{
        InPath:         in,
        Format:         FormatMarkdown,
        MarkdownMarp:   true,
        MarkdownImages: MarkdownImagesEmbed,
    })
    if err != nil {
        t.Fatal(err)
    }
    if !md.marp {
        t.Error("md.marp = false, want true")
    }
}
```

Also extend `fakeMD`:

```go
type fakeMD struct {
    got    []byte
    images MarkdownImageMode
    base   string
    marp   bool
    out    []byte
    err    error
}

func (f *fakeMD) Convert(html []byte, images MarkdownImageMode, base string, marp bool) ([]byte, error) {
    f.got = append(f.got, html...)
    f.images = images
    f.base = base
    f.marp = marp
    if f.err != nil {
        return nil, f.err
    }
    return f.out, nil
}
```

- [ ] **Step 2: Run, expect failure**

```bash
go test ./internal/worker/ -run TestPoolRunMarkdown -v
```

Expected: build failure (`undefined: Job.MarkdownMarp`,
`mdAdapter.Convert: too few arguments`).

- [ ] **Step 3: Add MarkdownMarp to types.go**

In `internal/worker/types.go`, extend the `Job` struct:

```go
type Job struct {
    InPath         string
    Format         Format
    Page           int
    DPI            float64
    Password       string
    MarkdownImages MarkdownImageMode
    MarkdownMarp   bool
}
```

- [ ] **Step 4: Update htmlToMarkdown interface**

In `internal/worker/iface.go`, replace:

```go
type htmlToMarkdown interface {
    Convert(html []byte, images MarkdownImageMode, base string) ([]byte, error)
}
```

with:

```go
type htmlToMarkdown interface {
    Convert(html []byte, images MarkdownImageMode, base string, marp bool) ([]byte, error)
}
```

- [ ] **Step 5: Update mdAdapter.Convert**

In `internal/worker/pool.go`, find `func (mdAdapter) Convert(...)` and
replace:

```go
func (mdAdapter) Convert(html []byte, mode MarkdownImageMode, base string) ([]byte, error) {
    var m mdconvpkg.ImageMode
    switch mode {
    case MarkdownImagesDrop:
        m = mdconvpkg.ImagesDrop
    default:
        m = mdconvpkg.ImagesEmbed
    }
    return mdconvpkg.ConvertWithBase(html, mdconvpkg.Options{Images: m}, base)
}
```

with:

```go
func (mdAdapter) Convert(html []byte, mode MarkdownImageMode, base string, marp bool) ([]byte, error) {
    var m mdconvpkg.ImageMode
    switch mode {
    case MarkdownImagesDrop:
        m = mdconvpkg.ImagesDrop
    default:
        m = mdconvpkg.ImagesEmbed
    }
    return mdconvpkg.ConvertWithBase(html, mdconvpkg.Options{Images: m, Marp: marp}, base)
}
```

- [ ] **Step 6: Update run_markdown.go**

In `internal/worker/run_markdown.go`, find:

```go
mdBytes, err := p.md.Convert(htmlBytes, job.MarkdownImages, filepath.Dir(htmlPath))
```

and replace with:

```go
mdBytes, err := p.md.Convert(htmlBytes, job.MarkdownImages, filepath.Dir(htmlPath), job.MarkdownMarp)
```

- [ ] **Step 7: Update pool_md_test.go and pool_extra_test.go callers**

Both files call `mdAdapter{}.Convert(...)` with three args. Add a fourth
`false`. In `pool_md_test.go`:

```go
got, err := mdAdapter{}.Convert([]byte("<p>hi</p>"), MarkdownImagesEmbed, ".", false)
```
and
```go
got, err := mdAdapter{}.Convert([]byte(`<p>x</p><img src="missing.png" alt="a"/><p>y</p>`), MarkdownImagesDrop, ".", false)
```

(Find both call sites and add `, false` before the closing paren.)

- [ ] **Step 8: Run, all worker tests pass**

```bash
go test ./internal/worker/... -v 2>&1 | tail -30
```

Expected: every test in the package passes including the new
`TestPoolRunMarkdownPropagatesMarpFlag`.

- [ ] **Step 9: Commit**

```bash
git add internal/worker/
git commit -m "$(cat <<'EOF'
feat(worker): plumb MarkdownMarp through Job → mdAdapter → mdconv

New bool field on worker.Job; the htmlToMarkdown seam gains a marp
parameter so production wiring can request Marp output. Tests assert
the flag flows through end-to-end via the fakeMD stub.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Auto-detect presentation Content-Types in handler

**Files:**
- Modify: `internal/server/handler_pdf.go`
- Modify: `internal/server/handler_markdown.go`
- Modify: `internal/server/handler_markdown_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/server/handler_markdown_test.go`:

```go
func TestMarkdownAutoEnablesMarpForPPTX(t *testing.T) {
    conv := &fakeConverter{body: []byte("# H"), mime: "text/markdown"}
    h := server.New(server.Deps{Conv: conv, MaxUploadBytes: 1 << 20})
    srv := httptest.NewServer(h.Routes())
    t.Cleanup(srv.Close)

    req, _ := http.NewRequest("POST", srv.URL+"/v1/convert/markdown", strings.NewReader("x"))
    req.Header.Set("Content-Type", "application/vnd.openxmlformats-officedocument.presentationml.presentation")
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        t.Fatal(err)
    }
    resp.Body.Close()
    if !conv.got.MarkdownMarp {
        t.Errorf("MarkdownMarp = false, want true (pptx input should auto-enable Marp)")
    }
}

func TestMarkdownAutoEnablesMarpForODP(t *testing.T) {
    conv := &fakeConverter{body: []byte("# H"), mime: "text/markdown"}
    h := server.New(server.Deps{Conv: conv, MaxUploadBytes: 1 << 20})
    srv := httptest.NewServer(h.Routes())
    t.Cleanup(srv.Close)

    req, _ := http.NewRequest("POST", srv.URL+"/v1/convert/markdown", strings.NewReader("x"))
    req.Header.Set("Content-Type", "application/vnd.oasis.opendocument.presentation")
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        t.Fatal(err)
    }
    resp.Body.Close()
    if !conv.got.MarkdownMarp {
        t.Errorf("MarkdownMarp = false, want true (odp input should auto-enable Marp)")
    }
}

func TestMarkdownDisablesMarpForDOCX(t *testing.T) {
    conv := &fakeConverter{body: []byte("# H"), mime: "text/markdown"}
    h := server.New(server.Deps{Conv: conv, MaxUploadBytes: 1 << 20})
    srv := httptest.NewServer(h.Routes())
    t.Cleanup(srv.Close)

    req, _ := http.NewRequest("POST", srv.URL+"/v1/convert/markdown", strings.NewReader("x"))
    req.Header.Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        t.Fatal(err)
    }
    resp.Body.Close()
    if conv.got.MarkdownMarp {
        t.Errorf("MarkdownMarp = true, want false (docx input should not enable Marp)")
    }
}
```

- [ ] **Step 2: Run, expect failure**

```bash
go test ./internal/server/ -run TestMarkdownAuto -v
```

Expected: failures on all three (`MarkdownMarp` is the zero value
because nothing sets it).

- [ ] **Step 3: Add isPresentationContentType to handler_pdf.go**

Append to `internal/server/handler_pdf.go` (next to `extensionFromContentType`):

```go
// isPresentationContentType reports whether ct identifies a presentation
// format LO recognises. Used by the markdown handler to auto-enable Marp
// output. Kept adjacent to extensionFromContentType so the two share a
// single source of truth for "what counts as a presentation".
func isPresentationContentType(ct string) bool {
    mt, _, err := mime.ParseMediaType(ct)
    if err != nil {
        return false
    }
    switch mt {
    case "application/vnd.openxmlformats-officedocument.presentationml.presentation",
        "application/vnd.oasis.opendocument.presentation",
        "application/vnd.ms-powerpoint":
        return true
    }
    return false
}
```

- [ ] **Step 4: Use it in handler_markdown.go**

In `internal/server/handler_markdown.go`, find the `worker.Job{}` literal
inside `convertMarkdown` and add the `MarkdownMarp` field:

```go
s.handleConversion(w, r, worker.Job{
    Format:         worker.FormatMarkdown,
    MarkdownImages: mode,
    MarkdownMarp:   isPresentationContentType(r.Header.Get("Content-Type")),
    Password:       r.Header.Get("X-Bi-Password"),
})
```

- [ ] **Step 5: Run, all pass**

```bash
go test ./internal/server/ -run TestMarkdown -v
```

Expected: existing markdown tests still pass; new auto-detect tests
pass. Also run the full server suite to be safe:

```bash
go test -race ./internal/server/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/server/
git commit -m "$(cat <<'EOF'
feat(server): auto-enable Marp output for pptx/odp/ppt uploads

isPresentationContentType (sibling of extensionFromContentType) maps
the three presentation Content-Types to true; convertMarkdown sets
Job.MarkdownMarp accordingly. .docx and friends keep flat-markdown
output. No new query params; no breaking changes.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: `bi convert -marp` flag + SubprocessConverter forwarding

**Files:**
- Modify: `cmd/bi/convert.go`
- Modify: `internal/server/subprocess.go`
- Modify: `internal/server/subprocess_test.go`

- [ ] **Step 1: Write the failing test**

In `internal/server/subprocess_test.go`, add at the bottom:

```go
func TestSubprocessConverter_MarpFlagForwarded(t *testing.T) {
    bin := fakeBinScript(t, `
out=""
next_out=0
saw_marp=0
for a in "$@"; do
  if [ "$next_out" = "1" ]; then out="$a"; next_out=0; continue; fi
  case "$a" in
    -out) next_out=1 ;;
    -marp) saw_marp=1 ;;
  esac
done
: > "$out"
if [ "$saw_marp" = "1" ]; then
  echo '{"mime":"text/markdown","total_pages":0}'
else
  echo '{"error":"internal","detail":"-marp flag not forwarded"}'
  exit 1
fi
`)
    in := filepath.Join(t.TempDir(), "in.pptx")
    os.WriteFile(in, []byte("x"), 0o600)
    c := &SubprocessConverter{BinPath: bin, Timeout: 5 * time.Second}
    res, err := c.Run(context.Background(), worker.Job{
        InPath: in, Format: worker.FormatMarkdown, MarkdownMarp: true,
    })
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { os.Remove(res.OutPath) })
    if res.MIME != "text/markdown" {
        t.Errorf("MIME = %q", res.MIME)
    }
}
```

- [ ] **Step 2: Run, expect failure**

```bash
go test ./internal/server/ -run TestSubprocessConverter_MarpFlagForwarded -v
```

Expected: the stub script exits 1 because `-marp` isn't passed; test
sees an error.

- [ ] **Step 3: Forward `-marp` from SubprocessConverter**

In `internal/server/subprocess.go`, find the args-building block (the
`-images` branch is right above where this goes):

```go
if job.Format == worker.FormatMarkdown {
    mode := "embed"
    if job.MarkdownImages == worker.MarkdownImagesDrop {
        mode = "drop"
    }
    args = append(args, "-images", mode)
}
```

Add right after it:

```go
if job.MarkdownMarp {
    args = append(args, "-marp")
}
```

- [ ] **Step 4: Add `-marp` flag to bi convert**

In `cmd/bi/convert.go`, inside `runConvert`, add a flag definition next
to the existing `images` flag:

```go
marp := fs.Bool("marp", false, "emit Marp-style markdown (markdown only)")
```

Update the `buildJob` call to pass it through:

```go
job, err := buildJob(*in, *format, *page, *dpi, *password, *images, *marp)
```

In the `buildJob` function signature, add `marp bool`:

```go
func buildJob(in, format string, page int, dpi float64, password, images string, marp bool) (worker.Job, error) {
    job := worker.Job{InPath: in, Password: password}
    switch strings.ToLower(format) {
    case "pdf":
        job.Format = worker.FormatPDF
    case "png":
        job.Format = worker.FormatPNG
        job.Page = page
        job.DPI = dpi
    case "markdown", "md":
        job.Format = worker.FormatMarkdown
        switch images {
        case "drop":
            job.MarkdownImages = worker.MarkdownImagesDrop
        case "embed", "":
            job.MarkdownImages = worker.MarkdownImagesEmbed
        default:
            return job, fmt.Errorf("invalid -images value %q", images)
        }
        job.MarkdownMarp = marp
    default:
        return job, fmt.Errorf("invalid -format value %q", format)
    }
    return job, nil
}
```

- [ ] **Step 5: Run, all pass**

```bash
go test -race ./...
```

Expected: all packages pass, including the new
`TestSubprocessConverter_MarpFlagForwarded` and the docx/pptx/odp
auto-detect cases from Task 5.

- [ ] **Step 6: Commit**

```bash
git add cmd/bi/ internal/server/
git commit -m "$(cat <<'EOF'
feat(cmd,server): plumb -marp from SubprocessConverter to bi convert

Server forwards Job.MarkdownMarp as a CLI flag; subprocess threads it
into worker.Job.MarkdownMarp before pool.Run. End-to-end Marp wiring
through the subprocess boundary.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Integration test against real LO + spec update

**Files:**
- Modify: `internal/worker/integration_test.go`
- Modify: `docs/superpowers/specs/2026-04-28-bi-http-api-design.md`

- [ ] **Step 1: Add the integration test**

Append to `internal/worker/integration_test.go`:

```go
func TestRealConversionMarkdownMarp(t *testing.T) {
    skipNoLOK(t)
    cfg := worker.Config{
        LOKPath:        os.Getenv("LOK_PATH"),
        Workers:        1,
        QueueDepth:     1,
        ConvertTimeout: 30 * time.Second,
    }
    p, err := worker.New(cfg)
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { _ = p.Close() })

    res, err := p.Run(context.Background(), worker.Job{
        InPath:         loadFixture(t, "health.pptx"),
        Format:         worker.FormatMarkdown,
        MarkdownImages: worker.MarkdownImagesEmbed,
        MarkdownMarp:   true,
    })
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { _ = os.Remove(res.OutPath) })

    body, err := os.ReadFile(res.OutPath)
    if err != nil {
        t.Fatal(err)
    }
    s := string(body)
    if !strings.HasPrefix(s, "---\nmarp: true\n---\n") {
        t.Errorf("output missing Marp front-matter: %.200q", s)
    }
    // health.pptx has 2 slides → exactly one internal --- separator.
    if c := strings.Count(s, "\n---\n"); c < 2 {
        // 1 from front-matter close + ≥1 between slides
        t.Errorf("expected ≥2 `---` lines (front-matter close + slide breaks); got %d in:\n%s", c, s)
    }
}
```

Add `"strings"` to the imports if it's not already there.

- [ ] **Step 2: Run locally (skipped without LOK_PATH)**

```bash
go test -tags=integration ./internal/worker/...
```

Expected: integration test SKIPs because `LOK_PATH` isn't set on the dev
host. The real validation comes from `make docker-test` in step 4.

- [ ] **Step 3: Update spec markdown route description**

In `docs/superpowers/specs/2026-04-28-bi-http-api-design.md`, find the
markdown row in the routes table:

```
| `/v1/convert/markdown?images=embed\|drop` | POST | document bytes      | `text/markdown`     | Pipeline: LO → HTML → `mdconv`. `images` defaults to `embed`. |
```

Replace with:

```
| `/v1/convert/markdown?images=embed\|drop` | POST | document bytes      | `text/markdown`     | Pipeline: LO → HTML → `mdconv`. `images` defaults to `embed`. Pptx/odp/ppt inputs auto-emit Marp markdown (front-matter + `---` slide separators). Other formats produce flat markdown unchanged. |
```

Also add a paragraph under `## Markdown pipeline (`internal/mdconv`)`:

```
### Marp output for presentations

When the request Content-Type identifies a presentation
(`application/vnd.openxmlformats-officedocument.presentationml.presentation`,
`application/vnd.oasis.opendocument.presentation`, or
`application/vnd.ms-powerpoint`), the handler sets `Job.MarkdownMarp = true`.
`mdconv` then prepends `---\nmarp: true\n---\n\n` and replaces horizontal-rule
slide markers (LO emits `<hr/>` between slides; html-to-markdown renders
them as `---`, `***`, or `* * *`) with normalised `---` separators. Empty
segments — produced by adjacent `<hr/>` markers in the source HTML — are
dropped so the output never contains adjacent separators. Speaker notes
are not extracted; LO's html filter does not surface them reliably.
```

- [ ] **Step 4: docker-test exercises the integration test**

```bash
make docker-test
```

Expected: green. The new `TestRealConversionMarkdownMarp` runs inside
the test image (LO present, LOK_PATH set) and asserts the front-matter
+ separator structure on the real fixture.

- [ ] **Step 5: Smoke-test deployed runtime image**

```bash
docker rm -f bi-smoke 2>/dev/null
docker build -t bi:dev .
docker run -d --name bi-smoke -p 8080:8080 bi:dev
until curl -fsS -o /dev/null http://localhost:8080/readyz 2>/dev/null; do sleep 2; done

# pptx → marp
curl -fsS -X POST \
  -H "Content-Type: application/vnd.openxmlformats-officedocument.presentationml.presentation" \
  --data-binary @testdata/health.pptx \
  http://localhost:8080/v1/convert/markdown
echo
# docx → flat (control)
curl -fsS -X POST \
  -H "Content-Type: application/vnd.openxmlformats-officedocument.wordprocessingml.document" \
  --data-binary @testdata/health.docx \
  http://localhost:8080/v1/convert/markdown

docker rm -f bi-smoke
```

Expected:
1. The pptx response starts with `---\nmarp: true\n---\n\n` and contains
   `\n---\n` between the two slides.
2. The docx response is plain markdown with no front-matter, no
   `---` separators.
3. Container `bi-smoke` is `Up` after both calls (no LO crash).

- [ ] **Step 6: Commit + push**

```bash
git add internal/worker/integration_test.go docs/superpowers/specs/
git commit -m "$(cat <<'EOF'
test(worker): integration test for pptx → Marp + spec note

Real-LO test loads testdata/health.pptx and asserts the response begins
with Marp front-matter and contains slide separators. Spec updated to
document the auto-detection contract.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git push -u origin feat/pptx-to-marp
```

---

## Task 8: Final verification + PR

- [ ] **Step 1: Re-run full local matrix**

```bash
go vet ./...
gofmt -s -l . | head
go test -race ./...
make cover-gate
```

Expected: vet clean, fmt empty, race-tests green, cover-gate green
(config 96+ / mdconv 90+ / server 90+ / worker 85+).

- [ ] **Step 2: Re-run docker-test**

```bash
make docker-test
```

Expected: image `bi:test` builds; full matrix passes inside the image.

- [ ] **Step 3: Open PR**

```bash
gh pr create --base main --title "feat: auto-emit Marp markdown for pptx/odp/ppt uploads" --body "$(cat <<'EOF'
## Summary

When a presentation file (.pptx / .odp / .ppt) is posted to
\`/v1/convert/markdown\`, the response is Marp-style markdown with
\`---\nmarp: true\n---\` front-matter and \`---\` slide separators.
Other input formats keep producing flat markdown unchanged. No new
route, no new query params.

Closes the user request from the chat thread on 2026-04-30.

## Detection

The handler inspects the request Content-Type and auto-enables Marp for
the three presentation MIME types. Single source of truth lives next
to \`extensionFromContentType\` in \`handler_pdf.go\`.

## Pipeline

\`mdconv\` post-processes the existing flat markdown:
- Prepend \`---\nmarp: true\n---\n\n\` front-matter
- Split on horizontal-rule lines (LO's \`<hr/>\` slide markers render as
  \`---\`, \`***\`, or \`* * *\`)
- Drop empty segments
- Rejoin with \`\n\n---\n\n\`

The image-embed/drop pipeline runs first so images survive correctly.

## Coverage

Four new mdconv golden pairs (simple, no-slides, empty-slide, with-image),
three handler tests (pptx/odp auto-on, docx auto-off), one subprocess
flag-forwarding test, one real-LO integration test on a programmatically
generated 2-slide \`testdata/health.pptx\`.

## Test plan

- [x] \`make test -race\`
- [x] \`make cover-gate\`
- [x] \`make docker-test\`
- [x] Smoke: pptx response starts with \`---\\nmarp: true\\n---\\n\` and
  contains a slide separator; docx response unchanged
- [x] Container stays \`Up\` after both round-trips
EOF
)"
```

---

## Self-review

**Spec coverage:** Each requirement in the spec maps to a task:

- pptx/odp/ppt auto-detection → Task 5 (`isPresentationContentType` + handler wiring + 3 tests)
- Front-matter shape (`---\nmarp: true\n---`) → Task 2 (`applyMarp` + `marp-simple` golden)
- Slide separator (`\n\n---\n\n`) → Task 2 (golden) + Task 3 (boundary cases)
- Image embed/drop interaction → Task 3 (`marp-with-image` golden)
- Empty-segment handling → Task 3 (`marp-empty-slide` golden)
- Single-slide handling → Task 3 (`marp-no-slides` golden)
- Plumbing through subprocess → Task 6
- Real-LO smoke against pptx → Task 7 + Task 1 fixture
- Spec doc update → Task 7 (step 3)

No gaps.

**Placeholder scan:** No "TBD", "TODO", "implement later", "similar to
Task N", or "add appropriate validation" in any step. Every code block
is complete.

**Type consistency:** `MarkdownMarp` is the field name everywhere
(types.go, run_markdown.go, handler_markdown.go, subprocess.go,
convert.go). The `htmlToMarkdown.Convert` signature is identical at
every call site (4 args including `marp bool`). The fakeMD has the
matching signature. The `bi convert -marp` flag name matches the
subprocess args. The mdconv field is `Options.Marp` and the helper is
`applyMarp`.

No issues found.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-04-30-pptx-to-marp.md`. Two execution options:

**1. Subagent-Driven (recommended)** — Fresh subagent per task with two-stage review. Higher quality, more agent dispatches.

**2. Inline Execution** — Execute tasks here using executing-plans, batched with checkpoints.

Which approach?
