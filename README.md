# bi

`bi` is a dockerized HTTP service that converts office documents (such as `.docx`, `.xlsx`, `.pptx`, `.odt`) into:

- PDF
- PNG (per-page render and thumbnails)
- Markdown

## Installation

### Prerequisites

- Go 1.25+
- LibreOffice installed on the host (for local execution and integration tests)
- `LOK_PATH` pointing to the LibreOffice `program/` directory (example: `/usr/lib/libreoffice/program`)

### Build from source

```bash
git clone https://github.com/julianshen/bi.git
cd bi
make build
```

This builds the binary at `./bin/bi`.

### Build with Docker

```bash
docker build -t bi:dev .
```

## Usage

### Run locally

```bash
export LOK_PATH=/usr/lib/libreoffice/program
make run
```

Or run the built binary directly:

```bash
./bin/bi
```

### Run with Docker

```bash
docker run --rm -p 8080:8080 -e LOK_PATH=/usr/lib/libreoffice/program bi:dev
```

### Development checks

```bash
make vet
make test
make test-integration
make cover
make cover-gate
```

## API usage

By default, the service listens on `:8080` and exposes conversion endpoints under `/v1`.

## HTTP API reference

Base URL: `http://localhost:8080`.

All conversion endpoints are `POST` and require a request body containing the source file bytes.

### Auth

- When `BI_API_TOKEN` is set, send `Authorization: Bearer <token>`.
- When `BI_API_TOKEN` is unset, conversion endpoints are public.

### Endpoints

| Method | Path | Description | Success response |
|---|---|---|---|
| `GET` | `/healthz` | Liveness probe. | `200 text/plain` |
| `GET` | `/readyz` | Readiness probe (includes conversion sanity check cache). | `200 text/plain` |
| `GET` | `/metrics` | Prometheus metrics endpoint. | `200 text/plain` |
| `POST` | `/v1/convert/pdf` | Convert office document to PDF. | `200 application/pdf` |
| `POST` | `/v1/convert/png` | Render one page or page grid as PNG. | `200 image/png` |
| `POST` | `/v1/thumbnail` | Generate thumbnail PNG (same options as PNG with lower default DPI). | `200 image/png` |
| `POST` | `/v1/convert/markdown` | Convert document to Markdown (optional OCR and image mode controls). | `200 text/markdown` |

### Common request headers (conversion endpoints)

- `Content-Type` (required): MIME type of uploaded input document.
- `Authorization` (required only if `BI_API_TOKEN` is configured): `Bearer <token>`.
- `X-Bi-Password` (optional): password for encrypted files.
- `X-Bi-Request-Id` (optional): caller-provided request ID.

### Common response headers (conversion endpoints)

- `X-Bi-Request-Id`: request correlation ID.
- `X-Total-Pages`: total source pages when known.
- `Content-Type`, `Content-Length`: output format and size.

### Query parameters

#### `POST /v1/convert/png` and `POST /v1/thumbnail`

- `page` (optional, integer): 1-based single page to render.
- `pages` (optional, list/ranges): multiple pages, e.g. `1,3-5`.
- `layout` (optional, requires `pages`): grid, e.g. `2x2`.
- `dpi` (optional, float `0.1` to `4.0`): render scale.
  - Default for `/v1/convert/png`: `1.0`
  - Default for `/v1/thumbnail`: `0.5`

`page` and `pages` are mutually exclusive.

#### `POST /v1/convert/markdown`

- `images` (optional): `embed` (default) or `drop`.
- `ocr` (optional): `auto` (default), `always`, or `never`.
- `ocr_lang` (optional): Tesseract language(s), default `auto`.

### Error behavior

- Invalid query parameter values return a Problem JSON error (`400`).
- Missing `Content-Type` returns an error (`400`).
- Auth failures return `401` when token auth is enabled.
- OCR-required requests can return `503` when OCR runtime is unavailable.

### Health and metrics

```bash
curl -sS http://localhost:8080/healthz
curl -sS http://localhost:8080/readyz
curl -sS http://localhost:8080/metrics
```

### Convert to PDF

Add this header when `BI_API_TOKEN` is configured:

```bash
-H "Authorization: Bearer $BI_API_TOKEN"
```

```bash
curl -sS \
  -X POST "http://localhost:8080/v1/convert/pdf" \
  -H "Content-Type: application/vnd.openxmlformats-officedocument.wordprocessingml.document" \
  --data-binary @testdata/simple.docx \
  -o out.pdf
```

### Convert to PNG

```bash
curl -sS \
  -X POST "http://localhost:8080/v1/convert/png?page=1&dpi=1.0" \
  -H "Content-Type: application/pdf" \
  --data-binary @testdata/multi-page.pdf \
  -o out.png
```

### Generate thumbnail

```bash
curl -sS \
  -X POST "http://localhost:8080/v1/thumbnail" \
  -H "Content-Type: application/pdf" \
  --data-binary @testdata/multi-page.pdf \
  -o thumb.png
```

### Convert to Markdown

```bash
curl -sS \
  -X POST "http://localhost:8080/v1/convert/markdown?images=embed&ocr=auto&ocr_lang=eng" \
  -H "Content-Type: application/vnd.openxmlformats-officedocument.wordprocessingml.document" \
  --data-binary @testdata/simple.docx \
  -o out.md
```

### Optional request/response headers

- `X-Bi-Password`: password for encrypted input documents.
- `X-Bi-Request-Id`: caller-provided request ID (otherwise server generates one).
- Response headers include `X-Bi-Request-Id` and, when available, `X-Total-Pages`.

## Project commands

The `Makefile` includes common tasks:

- `make build` – build `bin/bi`
- `make run` – run the service
- `make test` – unit tests
- `make test-integration` – integration tests (requires LibreOffice/`LOK_PATH`)
- `make cover` / `make cover-gate` – coverage reports and thresholds
- `make docker` / `make docker-test` – container build and test image build
