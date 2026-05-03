#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
invocation_dir="$(pwd)"
go_bin="${GO:-go}"
export GOCACHE="${GOCACHE:-$repo_root/.gocache}"
. "$repo_root/scripts/lib/common.sh"
dry_run=0
run_build=0
run_vet=0
run_unit=0
run_integration=0
run_coverage=0
run_fmt=0
selected=0
host_safe_tags='nolok noocr'
sample_files=()
sample_format="all"
sample_out_dir="$repo_root/out/script-samples"

usage() {
  cat <<'USAGE'
Usage: scripts/test.sh [options]

Runs the project test matrix with explicit, composable switches.

Options:
  --all           Run fmt, vet, build, unit, integration, and coverage checks.
  --default       Run fmt, vet, build, unit, and coverage checks. This is the default.
  --fmt           Check gofmt output without rewriting files.
  --build         Run go build ./... with host-safe nolok/noocr tags.
  --vet           Run go vet ./... with host-safe nolok/noocr tags.
  --unit          Run go test -race ./... with host-safe nolok/noocr tags.
  --integration   Run go test -race -tags=integration ./...
  --coverage      Run make cover-gate.
  --file PATH     Run conversion smoke checks against PATH. Repeat for multiple files.
  --format FORMAT Conversion format for --file: pdf, png, markdown, or all. Default: all.
  --samples-out DIR
                  Directory for conversion outputs. Default: out/script-samples.
  --dry-run       Print commands instead of executing them.
  -h, --help      Show this help.
USAGE
}

select_default() {
  run_fmt=1
  run_build=1
  run_vet=1
  run_unit=1
  run_coverage=1
}

select_all() {
  select_default
  run_integration=1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --all)
      select_all
      selected=1
      ;;
    --default)
      select_default
      selected=1
      ;;
    --fmt)
      run_fmt=1
      selected=1
      ;;
    --build)
      run_build=1
      selected=1
      ;;
    --vet)
      run_vet=1
      selected=1
      ;;
    --unit)
      run_unit=1
      selected=1
      ;;
    --integration)
      run_integration=1
      selected=1
      ;;
    --coverage)
      run_coverage=1
      selected=1
      ;;
    --file)
      if [[ $# -lt 2 ]]; then
        printf '%s\n' '--file requires a path' >&2
        exit 2
      fi
      sample_files+=("$(normalize_path "$2")")
      selected=1
      shift
      ;;
    --format)
      if [[ $# -lt 2 ]]; then
        printf '%s\n' '--format requires pdf, png, markdown, or all' >&2
        exit 2
      fi
      sample_format="$2"
      shift
      ;;
    --samples-out)
      if [[ $# -lt 2 ]]; then
        printf '%s\n' '--samples-out requires a directory' >&2
        exit 2
      fi
      sample_out_dir="$2"
      shift
      ;;
    --dry-run)
      dry_run=1
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      printf 'unknown option: %s\n\n' "$1" >&2
      usage >&2
      exit 2
      ;;
  esac
  shift
done

if [[ "$selected" -eq 0 ]]; then
  select_default
fi

if [[ "$dry_run" -eq 0 ]]; then
  mkdir -p "$GOCACHE"
fi

case "$sample_format" in
  pdf|png|markdown|all) ;;
  *)
    printf 'invalid --format %q; want pdf, png, markdown, or all\n' "$sample_format" >&2
    exit 2
    ;;
esac

if [[ "$sample_out_dir" != /* ]]; then
  sample_out_dir="$repo_root/$sample_out_dir"
fi

run() {
  local cmd="$1"
  printf '+ %s\n' "$cmd"
  if [[ "$dry_run" -eq 0 ]]; then
    (cd "$repo_root" && bash -c "$cmd")
  fi
}

run_sample_conversions() {
  if [[ "${#sample_files[@]}" -eq 0 ]]; then
    return
  fi
  if [[ "$dry_run" -eq 0 ]]; then
    mkdir -p "$sample_out_dir" "$repo_root/bin"
  else
    printf '+ mkdir -p %s %s\n' "$(shell_quote "$sample_out_dir")" "$(shell_quote "$repo_root/bin")"
  fi
  run "$go_bin build -tags=noocr -o bin/bi ./cmd/bi"
  local index=0
  local formats=()
  local listed_format
  while IFS= read -r listed_format; do
    formats+=("$listed_format")
  done < <(sample_formats "$sample_format")
  for input in "${sample_files[@]}"; do
    index=$((index + 1))
    if [[ "$dry_run" -eq 0 && ! -f "$input" ]]; then
      printf 'missing input file: %s\n' "$input" >&2
      exit 1
    fi
    local stem
    stem="$(sample_output_stem "$index" "$input")"
    local format ext out cmd
    for format in "${formats[@]}"; do
      ext="$(sample_ext "$format")"
      out="$sample_out_dir/$stem.$ext"
      cmd="$(conversion_cmd "bin/bi" "$input" "$out" "$format")"
      run "rm -f $(shell_quote "$out")"
      run "$cmd"
      if [[ "$dry_run" -eq 0 && ! -s "$out" ]]; then
        printf 'conversion produced no output: %s\n' "$out" >&2
        exit 1
      fi
    done
  done
}

if [[ "$run_fmt" -eq 1 ]]; then
  if [[ "$dry_run" -eq 1 ]]; then
    printf '+ test -z "$(gofmt -l .)"\n'
  else
    fmt_out="$(cd "$repo_root" && gofmt -l .)"
    if [[ -n "$fmt_out" ]]; then
      printf 'gofmt required for:\n%s\n' "$fmt_out" >&2
      exit 1
    fi
  fi
fi

if [[ "$run_vet" -eq 1 ]]; then
  run "$go_bin vet -tags=\"$host_safe_tags\" ./..."
fi

if [[ "$run_build" -eq 1 ]]; then
  run "$go_bin build -tags=\"$host_safe_tags\" ./..."
fi

if [[ "$run_unit" -eq 1 ]]; then
  run "$go_bin test -race -tags=\"$host_safe_tags\" ./..."
fi

if [[ "$run_integration" -eq 1 ]]; then
  run "$go_bin test -race -tags=integration ./..."
fi

if [[ "$run_coverage" -eq 1 ]]; then
  run "make cover-gate"
fi

run_sample_conversions
