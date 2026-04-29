# syntax=docker/dockerfile:1.7

# ─────────────────────────────────────────────────────────────────────────────
# Stage 1 — build the Go binary against the same glibc as the runtime image.
# cgo is required because golibreofficekit/lok wraps the LOK C ABI.
# ─────────────────────────────────────────────────────────────────────────────
FROM golang:1.25-bookworm AS build

ENV CGO_ENABLED=1 \
    GOFLAGS="-trimpath" \
    GOPROXY=https://proxy.golang.org,direct

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -ldflags="-s -w" -o /out/bi ./cmd/bi

# ─────────────────────────────────────────────────────────────────────────────
# Stage 1b — full test image: build toolchain + LibreOffice. Runs both unit
# tests and integration tests against a real LO install, so a passing
# `docker build --target test .` is a stronger guarantee than `go test ./...`
# on a host without LO.
#
# The integration-tagged tests skip when LOK_PATH is unset; we set it.
# ─────────────────────────────────────────────────────────────────────────────
FROM golang:1.25-bookworm AS test

ENV CGO_ENABLED=1 \
    GOFLAGS="-trimpath" \
    GOPROXY=https://proxy.golang.org,direct \
    LOK_PATH=/usr/lib/libreoffice/program

RUN apt-get update \
 && apt-get install -y --no-install-recommends \
        libreoffice-core \
        libreoffice-writer \
        libreoffice-calc \
        libreoffice-impress \
        libreoffice-draw \
        ca-certificates \
        fonts-liberation \
        fonts-dejavu-core \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Unit tests + integration tests + coverage gate. Any failure aborts the
# image build.
RUN go vet ./... \
 && gofmt -s -l . | (! grep .) \
 && go test -race ./... \
 && go test -tags=integration -race ./... \
 && make cover-gate

# ─────────────────────────────────────────────────────────────────────────────
# Stage 2 — runtime image with LibreOffice installed.
#
# We pull the per-app split packages instead of the meta `libreoffice` package
# to avoid the GUI front-end. LOK loads libsofficeapp.so / libmergedlo.so
# from /usr/lib/libreoffice/program at runtime via dlopen, so the full
# LibreOffice tree must be present — scratch / distroless will not work.
# ─────────────────────────────────────────────────────────────────────────────
FROM debian:bookworm-slim AS runtime

RUN apt-get update \
 && apt-get install -y --no-install-recommends \
        libreoffice-core \
        libreoffice-writer \
        libreoffice-calc \
        libreoffice-impress \
        libreoffice-draw \
        ca-certificates \
        fonts-liberation \
        fonts-dejavu-core \
 && rm -rf /var/lib/apt/lists/*

ENV LOK_PATH=/usr/lib/libreoffice/program \
    BI_LISTEN_ADDR=:8080

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
