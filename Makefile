SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c

GO              ?= go
PKG             ?= ./...
COVERAGE_FILE   ?= coverage.out
COVERAGE_MIN    ?= 90

.PHONY: build vet test test-integration cover cover-gate fmt tidy docker run clean

build:
	$(GO) build -o bin/bi ./cmd/bi

vet:
	$(GO) vet $(PKG)

test:
	$(GO) test -race $(PKG)

# Integration tests hit a real LibreOffice install; they skip when LOK_PATH is unset.
test-integration:
	$(GO) test -race -tags=integration $(PKG)

cover:
	$(GO) test -covermode=atomic -coverprofile=$(COVERAGE_FILE) $(PKG)
	$(GO) tool cover -func=$(COVERAGE_FILE) | tail -n 1

# Fail if total coverage drops below COVERAGE_MIN.
#
# The gate is calculated against testable Go code:
#   -coverpkg=./internal/...  excludes cmd/bi bootstrap (binds a port,
#                             not amenable to unit tests).
#   -tags=nolok               swaps lok_adapter.go (cgo pass-through to
#                             LibreOffice) for lok_adapter_nolok.go (stub),
#                             so the cgo trampolines that can only run with
#                             a real LO install are not in the profile.
# Real LO coverage is exercised by `make test-integration` against a host
# with LibreOffice installed.
cover-gate:
	$(GO) test -tags=nolok -covermode=atomic -coverpkg=./internal/... -coverprofile=$(COVERAGE_FILE) ./internal/...
	@pct=$$($(GO) tool cover -func=$(COVERAGE_FILE) | tail -n 1 | awk '{print $$3}' | tr -d '%'); \
	awk -v p=$$pct -v m=$(COVERAGE_MIN) 'BEGIN { if (p+0 < m+0) { printf "coverage %.1f%% < %d%%\n", p, m; exit 1 } else { printf "coverage %.1f%% >= %d%%\n", p, m } }'

fmt:
	gofmt -s -w .

tidy:
	$(GO) mod tidy

docker:
	docker build -t bi:dev .

run: build
	./bin/bi

clean:
	rm -rf bin out $(COVERAGE_FILE) coverage.html
