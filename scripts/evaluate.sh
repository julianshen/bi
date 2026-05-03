#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
invocation_dir="$(pwd)"
go_bin="${GO:-go}"
export GOCACHE="${GOCACHE:-$repo_root/.gocache}"
. "$repo_root/scripts/lib/common.sh"
dry_run=0
quick=0
files_only=0
out_dir=""
host_safe_tags='nolok noocr'
failures=0
sample_files=()
sample_format="all"

usage() {
  cat <<'USAGE'
Usage: scripts/evaluate.sh [options]

Runs an evaluation pass and writes logs plus coverage artifacts to a results directory.

Options:
  --out DIR       Directory for evaluation artifacts. Defaults to results/evaluation-<timestamp>.
  --quick         Skip integration and Docker checks; useful on hosts without LibreOffice.
  --files-only    Only run --file conversions; skip project tests and coverage checks.
  --file PATH     Run conversion evaluation against PATH. Repeat for multiple files.
  --format FORMAT Conversion format for --file: pdf, png, markdown, or all. Default: all.
  --dry-run       Print commands instead of executing them.
  -h, --help      Show this help.
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --out)
      if [[ $# -lt 2 ]]; then
        printf '%s\n' '--out requires a directory' >&2
        exit 2
      fi
      out_dir="$2"
      shift
      ;;
    --quick)
      quick=1
      ;;
    --files-only)
      files_only=1
      ;;
    --file)
      if [[ $# -lt 2 ]]; then
        printf '%s\n' '--file requires a path' >&2
        exit 2
      fi
      sample_files+=("$(normalize_path "$2")")
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

if [[ -z "$out_dir" ]]; then
  out_dir="$repo_root/results/evaluation-$(date -u +%Y%m%dT%H%M%SZ)"
elif [[ "$out_dir" != /* ]]; then
  out_dir="$repo_root/$out_dir"
fi

case "$sample_format" in
  pdf|png|markdown|all) ;;
  *)
    printf 'invalid --format %q; want pdf, png, markdown, or all\n' "$sample_format" >&2
    exit 2
    ;;
esac

if [[ "$files_only" -eq 1 && "${#sample_files[@]}" -eq 0 ]]; then
  printf '%s\n' '--files-only requires at least one --file' >&2
  exit 2
fi

run() {
  local cmd="$1"
  printf '+ %s\n' "$cmd"
  if [[ "$dry_run" -eq 0 ]]; then
    (cd "$repo_root" && bash -c "$cmd")
  fi
}

run_logged() {
  local name="$1"
  local cmd="$2"
  printf '+ %s\n' "$cmd"
  if [[ "$dry_run" -eq 0 ]]; then
    if (cd "$repo_root" && bash -c "$cmd") 2>&1 | tee "$out_dir/$name.log"; then
      printf '%s PASS\n' "$name" >> "$out_dir/status.txt"
    else
      local status="$?"
      printf '%s FAIL %s\n' "$name" "$status" >> "$out_dir/status.txt"
      failures=1
    fi
  fi
}

run_file_evaluations() {
  if [[ "${#sample_files[@]}" -eq 0 ]]; then
    return
  fi
  local samples_dir="$out_dir/files"
  if [[ "$dry_run" -eq 0 ]]; then
    mkdir -p "$samples_dir"
  else
    run "mkdir -p $(shell_quote "$samples_dir")"
  fi

  local existing_files=()
  if [[ "$dry_run" -eq 1 ]]; then
    existing_files=("${sample_files[@]}")
  else
    for input in "${sample_files[@]}"; do
      if [[ -f "$input" ]]; then
        existing_files+=("$input")
      else
        printf 'sample-file FAIL missing %s\n' "$input" >> "$out_dir/status.txt"
        failures=1
      fi
    done
    if [[ "${#existing_files[@]}" -eq 0 ]]; then
      return
    fi
  fi

  local eval_bin="$out_dir/bi-eval"
  run "rm -f $(shell_quote "$eval_bin")"
  run_logged "build-bi" "$go_bin build -tags=noocr -o $(shell_quote "$eval_bin") ./cmd/bi"
  if [[ "$dry_run" -eq 0 && ! -x "$eval_bin" ]]; then
    printf 'file-evaluation FAIL build-unavailable\n' >> "$out_dir/status.txt"
    failures=1
    return
  fi
  local index=0
  local formats=()
  local listed_format
  while IFS= read -r listed_format; do
    formats+=("$listed_format")
  done < <(sample_formats "$sample_format")
  for input in "${existing_files[@]}"; do
    index=$((index + 1))
    local stem
    stem="$(sample_output_stem "$index" "$input")"
    local format ext output name cmd
    for format in "${formats[@]}"; do
      ext="$(sample_ext "$format")"
      output="$samples_dir/$stem.$ext"
      name="file-$stem-$format"
      cmd="$(conversion_cmd "$eval_bin" "$input" "$output" "$format")"
      run "rm -f $(shell_quote "$output")"
      if run_logged "$name" "$cmd" && [[ "$dry_run" -eq 0 && ! -s "$output" ]]; then
        printf '%s FAIL empty-output\n' "$name" >> "$out_dir/status.txt"
        failures=1
      fi
    done
  done
}

if [[ "$dry_run" -eq 0 ]]; then
  mkdir -p "$out_dir" "$GOCACHE"
  : > "$out_dir/status.txt"
else
  run "mkdir -p $(shell_quote "$out_dir")"
fi

if [[ "$files_only" -eq 0 ]]; then
  run_logged "unit" "$go_bin test -race -tags=\"$host_safe_tags\" ./..."
  coverage_out="$out_dir/coverage.out"
  run_logged "coverage" "$go_bin test -covermode=atomic -tags=\"$host_safe_tags\" -coverprofile=$(shell_quote "$coverage_out") ./..."
  if [[ "$dry_run" -eq 1 || -f "$out_dir/coverage.out" ]]; then
    run_logged "coverage-func" "$go_bin tool cover -func=$(shell_quote "$coverage_out")"
  fi
  run_logged "coverage-gate" "make cover-gate"

  if [[ "$quick" -eq 0 ]]; then
    run_logged "integration" "$go_bin test -race -tags=integration ./..."
    run_logged "docker-test" "make docker-test"
  fi
fi

run_file_evaluations

if [[ "$dry_run" -eq 0 ]]; then
  {
    printf 'evaluation_dir=%s\n' "$out_dir"
    printf 'quick=%s\n' "$quick"
    printf 'files_only=%s\n' "$files_only"
    printf 'sample_files=%s\n' "${#sample_files[@]}"
    printf 'sample_format=%s\n' "$sample_format"
    printf 'git_commit='
    git -C "$repo_root" rev-parse HEAD
    printf 'coverage_total='
    if [[ -f "$out_dir/coverage-func.log" ]]; then
      awk '/^total:/ {print $3}' "$out_dir/coverage-func.log"
    else
      printf 'unavailable\n'
    fi
    printf 'failures=%s\n' "$failures"
  } > "$out_dir/summary.txt"
  printf 'evaluation artifacts written to %s\n' "$out_dir"
  exit "$failures"
fi
