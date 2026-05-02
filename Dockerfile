# syntax=docker/dockerfile:1.7

# ─────────────────────────────────────────────────────────────────────────────
# Stage 1 — build the Go binary against Ubuntu glibc for PDF import support.
# cgo is required because golibreofficekit/lok wraps the LOK C ABI.
# Build on Ubuntu so the binary is compatible with PDF support libraries.
# ─────────────────────────────────────────────────────────────────────────────
FROM ubuntu:24.04 AS build

ENV CGO_ENABLED=1 \
    GOFLAGS="-trimpath" \
    GOPROXY=https://proxy.golang.org,direct

RUN apt-get update \
 && apt-get install -y --no-install-recommends \
        golang-go \
        build-essential \
        ca-certificates \
        pkg-config \
        libx11-dev \
        libtesseract-dev \
        libleptonica-dev \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -ldflags="-s -w" -o /out/bi ./cmd/bi

# ─────────────────────────────────────────────────────────────────────────────
# Stage 2 — runtime image with LibreOffice installed.
#
# We pull the per-app split packages instead of the meta `libreoffice` package
# to avoid the GUI front-end. LOK loads libsofficeapp.so / libmergedlo.so
# from /usr/lib/libreoffice/program at runtime via dlopen, so the full
# LibreOffice tree must be present — scratch / distroless will not work.
#
# Note: the test pipeline (vet/gofmt/race/integration/cover-gate) lives in
# Dockerfile.test, NOT here. Keeping it out of this Dockerfile prevents the
# legacy Docker builder (DOCKER_BUILDKIT=0) from executing the test matrix
# during a plain `docker build .` — which would happen because the legacy
# builder walks every stage in the file regardless of --target.
# ─────────────────────────────────────────────────────────────────────────────
FROM ubuntu:24.04 AS runtime

RUN apt-get update \
 && apt-get install -y --no-install-recommends \
        libreoffice-core \
        libreoffice-writer \
        libreoffice-calc \
        libreoffice-impress \
        libreoffice-draw \
        media-types \
        ca-certificates \
        fonts-liberation \
        fonts-dejavu-core \
        tesseract-ocr \
        tesseract-ocr-eng \
        tesseract-ocr-jpn \
        tesseract-ocr-chi-sim \
        tesseract-ocr-chi-tra \
        tesseract-ocr-osd \
        libtesseract5 \
        libleptonica-dev \
 && rm -rf /var/lib/apt/lists/*

ENV LOK_PATH=/usr/lib/libreoffice/program \
    BI_LISTEN_ADDR=:8080 \
    GODEBUG=asyncpreemptoff=1 \
    BI_OCR_TESSDATA=/usr/share/tesseract-ocr/5/tessdata \
    TESSDATA_PREFIX=/usr/share/tesseract-ocr/5/tessdata
# GODEBUG=asyncpreemptoff=1 is required: LibreOffice installs a SIGURG handler
# without SA_ONSTACK, which crashes the Go runtime (Go uses SIGURG for goroutine
# preemption since 1.14). Disabling async preemption is the standard workaround
# for cgo programs that load LO. Slight scheduling-fairness cost is acceptable.

# Run as a non-root user. Conversions write to /tmp; HOME must be writable
# because LibreOffice spawns user-profile directories on first call.
RUN useradd --system --create-home --home-dir /home/bi --uid 10001 bi
USER bi
WORKDIR /home/bi

COPY --from=build /out/bi /usr/local/bin/bi

EXPOSE 8080

# Healthcheck performs a real round-trip conversion (per CLAUDE.md): a
# TCP-level probe will not catch a broken LibreOffice install, which is the
# most likely production failure mode for this service.
HEALTHCHECK --interval=30s --timeout=15s --start-period=20s --retries=3 \
    CMD bi healthcheck || exit 1

ENTRYPOINT ["bi"]
CMD ["serve"]
