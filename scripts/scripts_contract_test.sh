#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

assert_executable() {
  local path="$1"
  [[ -f "$path" ]] || fail "missing $path"
  [[ -x "$path" ]] || fail "not executable: $path"
}

assert_contains() {
  local haystack="$1"
  local needle="$2"
  [[ "$haystack" == *"$needle"* ]] || fail "expected output to contain: $needle"
}

assert_not_contains() {
  local haystack="$1"
  local needle="$2"
  [[ "$haystack" != *"$needle"* ]] || fail "expected output not to contain: $needle"
}

assert_fails() {
  if "$@" >"$tmp_dir/out" 2>"$tmp_dir/err"; then
    fail "expected command to fail: $*"
  fi
}

test_script="$repo_root/scripts/test.sh"
evaluate_script="$repo_root/scripts/evaluate.sh"

assert_executable "$test_script"
assert_executable "$evaluate_script"

test_help="$("$test_script" --help)"
assert_contains "$test_help" "Usage:"
assert_contains "$test_help" "--unit"
assert_contains "$test_help" "--integration"
assert_contains "$test_help" "--coverage"
assert_contains "$test_help" "--file"
assert_contains "$test_help" "--format"

eval_help="$("$evaluate_script" --help)"
assert_contains "$eval_help" "Usage:"
assert_contains "$eval_help" "--out"
assert_contains "$eval_help" "--quick"
assert_contains "$eval_help" "--file"
assert_contains "$eval_help" "--format"
assert_contains "$eval_help" "--files-only"

test_plan="$("$test_script" --dry-run --unit --coverage)"
assert_contains "$test_plan" "go test -race -tags=\"nolok noocr\" ./..."
assert_contains "$test_plan" "make cover-gate"

eval_plan="$("$evaluate_script" --dry-run --quick)"
assert_contains "$eval_plan" "go test -race -tags=\"nolok noocr\" ./..."
assert_contains "$eval_plan" "go test -covermode=atomic -tags=\"nolok noocr\" -coverprofile="

file_plan="$("$evaluate_script" --dry-run --quick --file testdata/simple.docx --format pdf)"
assert_contains "$file_plan" "convert -in"
assert_contains "$file_plan" "-format pdf"
assert_contains "$file_plan" "testdata/simple.docx"

files_only_plan="$("$evaluate_script" --dry-run --files-only --file testdata/simple.docx --format pdf)"
assert_contains "$files_only_plan" "convert -in"
assert_not_contains "$files_only_plan" "go test -race"

assert_fails "$evaluate_script" --files-only
assert_contains "$(cat "$tmp_dir/err")" "--files-only requires at least one --file"

test_file_plan="$("$test_script" --dry-run --file testdata/simple.docx --format markdown)"
assert_contains "$test_file_plan" "mkdir -p"
assert_contains "$test_file_plan" "/bin"
assert_contains "$test_file_plan" "bi convert -in"
assert_contains "$test_file_plan" "-format markdown"
assert_contains "$test_file_plan" "-ocr=never"
assert_contains "$test_file_plan" "rm -f"

collision_plan="$("$evaluate_script" --dry-run --files-only --file a/report.docx --file b/report.docx --format pdf)"
assert_contains "$collision_plan" "001-report.pdf"
assert_contains "$collision_plan" "002-report.pdf"
assert_contains "$collision_plan" "rm -f"
assert_contains "$collision_plan" "mkdir -p"

quoted_out_plan="$("$evaluate_script" --dry-run --quick --out "results/eval with space")"
assert_contains "$quoted_out_plan" "coverage.out"
assert_contains "$quoted_out_plan" "\\ "

printf 'scripts contract ok\n'
