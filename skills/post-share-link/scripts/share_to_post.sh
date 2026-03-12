#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
POST_CLI="$SCRIPT_DIR/../post-cli"
POST_CLI_URL="https://raw.githubusercontent.com/mirtlecn/post-go/refs/heads/master/post-cli"

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

random_suffix() {
  LC_ALL=C tr -dc 'a-z0-9' </dev/urandom 2>/dev/null | awk '{ print substr($0, 1, 6); exit }'
}

slugify() {
  _value=$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')
  _value=$(printf '%s' "$_value" | sed 's#https\{0,1\}://##g')
  _value=$(printf '%s' "$_value" | sed 's/[^a-z0-9]/-/g; s/-\{2,\}/-/g; s/^-//; s/-$//')
  _value=$(printf '%s' "$_value" | cut -c 1-48)
  _value=$(printf '%s' "$_value" | sed 's/-$//')
  printf '%s' "$_value"
}

first_nonempty_line() {
  printf '%s\n' "$1" | sed -n '/[^[:space:]]/ { s/^[[:space:]]*//; s/[[:space:]]*$//; p; q; }'
}

guess_slug_source() {
  _content="$1"
  _convert="$2"

  case "$mode" in
    file)
      if [ "$_convert" = "file" ]; then
        basename "$file"
        return 0
      fi
      ;;
  esac

  if [ "$_convert" = "url" ] || printf '%s' "$_content" | grep -Eq '^https?://'; then
    _url=$(printf '%s' "$_content" | head -n 1 | sed 's/[?#].*$//')
    _host=$(printf '%s' "$_url" | sed -E 's#^https?://([^/]+).*$#\1#; s/^www\.//')
    _path=$(printf '%s' "$_url" | sed -E 's#^https?://[^/]+/?##; s#/$##')
    _last=$(printf '%s' "$_path" | awk -F/ 'NF { print $NF }')
    [ -n "$_host" ] && [ -n "$_last" ] && {
      printf '%s %s' "$_host" "$_last"
      return 0
    }
    [ -n "$_host" ] && {
      printf '%s' "$_host"
      return 0
    }
  fi

  if [ "$_convert" = "md2html" ] || printf '%s\n' "$_content" | grep -Eq '^[[:space:]]*#'; then
    _md_title=$(printf '%s\n' "$_content" | sed -n 's/^[[:space:]]*#\{1,6\}[[:space:]]*//p' | sed -n '/[^[:space:]]/ { p; q; }')
    [ -n "$_md_title" ] && {
      printf '%s' "$_md_title"
      return 0
    }
  fi

  if [ "$_convert" = "html" ] || printf '%s' "$_content" | grep -Eqi '<(html|head|body|title|h1)\b'; then
    _html_title=$(printf '%s' "$_content" | sed -n 's/.*<[Tt][Ii][Tt][Ll][Ee]>\(.*\)<\/[Tt][Ii][Tt][Ll][Ee]>.*/\1/p' | sed -n '/[^[:space:]]/ { p; q; }')
    [ -z "$_html_title" ] && _html_title=$(printf '%s' "$_content" | sed -n 's/.*<[Hh]1[^>]*>\(.*\)<\/[Hh]1>.*/\1/p' | sed -n '/[^[:space:]]/ { p; q; }')
    [ -n "$_html_title" ] && {
      printf '%s' "$_html_title"
      return 0
    }
  fi

  first_nonempty_line "$_content"
}

build_auto_slug() {
  _content="$1"
  _convert="$2"
  _source=$(guess_slug_source "$_content" "$_convert")
  _slug=$(slugify "$_source")
  if [ -n "$_slug" ]; then
    printf '%s' "$_slug"
    return 0
  fi
  random_suffix
}

is_conflict_error() {
  printf '%s' "$1" | grep -Eqi 'already exists|conflict|choose another path|overwrite'
}

run_post() {
  _stdout=$(mktemp) || die "failed to create temp file"
  _stderr=$(mktemp) || die "failed to create temp file"
  if "$@" >"$_stdout" 2>"$_stderr"; then
    RUN_POST_STDOUT=$(cat "$_stdout")
    RUN_POST_STDERR=$(cat "$_stderr")
    rm -f "$_stdout" "$_stderr"
    return 0
  fi
  RUN_POST_STDOUT=$(cat "$_stdout")
  RUN_POST_STDERR=$(cat "$_stderr")
  rm -f "$_stdout" "$_stderr"
  return 1
}

ensure_post_cli() {
  if [ -x "$POST_CLI" ]; then
    return 0
  fi

  command -v curl >/dev/null 2>&1 || die "curl is required to download post-cli"

  printf 'info: downloading post-cli to %s\n' "$POST_CLI" >&2
  curl -fsSL "$POST_CLI_URL" -o "$POST_CLI" || die "failed to download post-cli from $POST_CLI_URL"
  chmod +x "$POST_CLI" || die "failed to make post-cli executable: $POST_CLI"
}

mode=""
text=""
file=""
slug=""
ttl=""
convert=""
update=0
export_json=0

while [ $# -gt 0 ]; do
  case "$1" in
    --text)
      [ $# -ge 2 ] || die "--text requires a value"
      mode="text"
      text="$2"
      shift 2
      ;;
    --file)
      [ $# -ge 2 ] || die "--file requires a path"
      mode="file"
      file="$2"
      shift 2
      ;;
    --clipboard)
      mode="clipboard"
      shift
      ;;
    --slug)
      [ $# -ge 2 ] || die "--slug requires a value"
      slug="$2"
      shift 2
      ;;
    --ttl)
      [ $# -ge 2 ] || die "--ttl requires a value in minutes"
      ttl="$2"
      shift 2
      ;;
    --convert)
      [ $# -ge 2 ] || die "--convert requires a value"
      convert="$2"
      shift 2
      ;;
    --update)
      update=1
      shift
      ;;
    --export)
      export_json=1
      shift
      ;;
    *)
      die "unknown option: $1"
      ;;
  esac
done

[ -n "${POST_HOST:-}" ] || die "POST_HOST is not set; please export POST_HOST='https://your-post-host'"
[ -n "${POST_TOKEN:-}" ] || die "POST_TOKEN is not set; please export POST_TOKEN='your-token'"

ensure_post_cli

case "$mode" in
  text)
    content="$text"
    ;;
  file)
    [ -f "$file" ] || die "file not found: $file"
    if [ "$convert" = "file" ]; then
      content=$(basename "$file")
    else
      content=$(cat "$file")
    fi
    ;;
  clipboard)
    if command -v pbpaste >/dev/null 2>&1; then
      content=$(pbpaste)
    elif command -v xclip >/dev/null 2>&1; then
      content=$(xclip -selection clipboard -o)
    elif command -v xsel >/dev/null 2>&1; then
      content=$(xsel --clipboard --output)
    else
      die "clipboard read tool not found"
    fi
    ;;
  *)
    die "one of --text, --file, or --clipboard is required"
    ;;
esac

[ -n "$slug" ] || slug=$(build_auto_slug "$content" "$convert")

base_slug="$slug"
attempt=1
max_attempts=5

while :; do
  set -- "$POST_CLI" new -y
  [ -n "$slug" ] && set -- "$@" -s "$slug"
  [ -n "$ttl" ] && set -- "$@" -t "$ttl"
  [ "$update" -eq 1 ] && set -- "$@" -u
  [ "$export_json" -eq 1 ] && set -- "$@" -x
  [ -n "$convert" ] && set -- "$@" -c "$convert"

  rc=0
  case "$mode" in
    text)
      if run_post "$@" "$text"; then
        rc=0
      else
        rc=1
      fi
      ;;
    file)
      if run_post "$@" -f "$file"; then
        rc=0
      else
        rc=1
      fi
      ;;
    clipboard)
      if run_post "$@"; then
        rc=0
      else
        rc=1
      fi
      ;;
  esac

  if [ "$rc" -eq 0 ]; then
    printf '%s\n' "$RUN_POST_STDOUT"
    exit 0
  fi

  if [ -n "${slug:-}" ] && [ "$update" -eq 0 ] && [ "$attempt" -lt "$max_attempts" ] && is_conflict_error "$RUN_POST_STDERR"; then
    attempt=$((attempt + 1))
    if [ "$attempt" -le 3 ]; then
      slug="${base_slug}-${attempt}"
    else
      slug="${base_slug}-$(random_suffix)"
    fi
    continue
  fi

  if [ -n "$RUN_POST_STDERR" ]; then
    printf '%s\n' "$RUN_POST_STDERR" >&2
  fi
  break
done

exit 1
