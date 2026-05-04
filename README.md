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

### Authentication

If `BI_API_TOKEN` is set, pass it as a bearer token:

```bash
-H "Authorization: Bearer $BI_API_TOKEN"
```

If `BI_API_TOKEN` is not set, conversion endpoints are unauthenticated. In the examples below, omit the `Authorization` header when auth is disabled.

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
