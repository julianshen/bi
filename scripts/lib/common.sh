#!/usr/bin/env bash

shell_quote() {
  printf '%q' "$1"
}

normalize_path() {
  local path="$1"
  if [[ "$path" == /* ]]; then
    printf '%s' "$path"
  else
    printf '%s/%s' "$invocation_dir" "$path"
  fi
}

sample_formats() {
  case "$1" in
    all)
      printf '%s\n' pdf png markdown
      ;;
    *)
      printf '%s\n' "$1"
      ;;
  esac
}

sample_ext() {
  case "$1" in
    pdf) printf 'pdf' ;;
    png) printf 'png' ;;
    markdown) printf 'md' ;;
  esac
}

sample_output_stem() {
  local index="$1"
  local input="$2"
  local base stem
  base="$(basename "$input")"
  stem="${base%.*}"
  printf '%03d-%s' "$index" "$stem"
}

conversion_cmd() {
  local bin="$1"
  local input="$2"
  local output="$3"
  local format="$4"
  local cmd

  cmd="$(shell_quote "$bin") convert -in $(shell_quote "$input") -out $(shell_quote "$output") -format $format -timeout 120s"
  if [[ "$format" == "markdown" ]]; then
    cmd="$cmd -ocr=never"
  fi
  printf '%s' "$cmd"
}
