SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c

GO              ?= go
PKG             ?= ./...
COVERAGE_FILE   ?= coverage.out
COVERAGE_MIN    ?= 90

.PHONY: build vet test test-integration cover cover-gate fmt tidy docker docker-test run clean

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

# Fail if any internal/* package drops below COVERAGE_MIN.
#
# We measure per-package self-coverage and require every package to clear
# the bar, rather than computing a cross-package merged percentage:
#   -coverpkg=./internal/... merge across multiple test binaries proved
#   unreliable in practice (Go writes separate profiles per package and
#   the merge attributes differently than the per-binary self-coverage
#   reports). Per-package gates are the more honest signal.
#
# -tags=nolok swaps lok_adapter.go (cgo pass-through to LibreOffice) for
# lok_adapter_nolok.go (stub), so the cgo trampolines that can only run
# with a real LO install are not in the profile. Real LO coverage is
# exercised by `make test-integration` against a host with LibreOffice
# installed.
cover-gate:
	@fail=0; \
	for entry in config:90 mdconv:90 server:90 worker:85; do \
	    pkg=$${entry%:*}; min=$${entry#*:}; \
	    out=$$( $(GO) test -tags=nolok -covermode=atomic -coverpkg=./internal/$$pkg/ ./internal/$$pkg/ 2>&1 | tail -1 ); \
	    pct=$$( echo "$$out" | awk '{for(i=1;i<=NF;i++) if($$i=="coverage:") print $$(i+1)}' | tr -d '%' ); \
	    awk -v p=$$pct -v m=$$min -v pkg=$$pkg 'BEGIN { \
	        if (p+0 < m+0) { printf "%-8s %.1f%% < %d%% FAIL\n", pkg, p, m; exit 1 } \
	        else            { printf "%-8s %.1f%% >= %d%%\n",   pkg, p, m } \
	    }' || fail=1; \
	done; \
	exit $$fail
# worker's threshold is 85% rather than 90%: the run_*.go conversion paths
# have ~6 filesystem-error branches (CreateTemp / Write / Close failures)
# that need OS-level injection to exercise. Real LO coverage from
# make test-integration brings the integrated number well above 90%.

fmt:
	gofmt -s -w .

tidy:
	$(GO) mod tidy

docker:
	docker build -t bi:dev .

# Run the full test matrix (vet/gofmt/integration/cover-gate) inside a
# container that has LibreOffice installed. Useful when LO isn't available
# on the dev host. A passing build proves the cgo path works against a
# real LO install. Lives in a separate Dockerfile.test so plain
# `docker build .` doesn't trigger it on legacy Docker builders.
docker-test:
	docker build -f Dockerfile.test -t bi:test .

run: build
	./bin/bi

clean:
	rm -rf bin out $(COVERAGE_FILE) coverage.html
