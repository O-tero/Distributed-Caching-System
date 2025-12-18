#!/usr/bin/env bash
# list-endpoints.sh - Extract Encore endpoints from source
#
# Usage:
#   ./scripts/list-endpoints.sh
#   ./scripts/list-endpoints.sh --include-private
#   ./scripts/list-endpoints.sh --json
#
# Output (text mode):
#   METHOD PATH

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

INCLUDE_PRIVATE=0
OUTPUT_FORMAT="text" # text|json

usage() {
  cat <<EOF
Usage: $0 [options]

Options:
  --include-private   Include //encore:api private endpoints
  --json              Emit JSON array: [{"method":"GET","path":"/..."}, ...]
  --help, -h          Show help
EOF
  exit 0
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --include-private)
      INCLUDE_PRIVATE=1
      shift
      ;;
    --json)
      OUTPUT_FORMAT="json"
      shift
      ;;
    --help|-h)
      usage
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage
      ;;
  esac
done

# Collect endpoints. Encore annotations look like:
#   //encore:api public method=GET path=/foo/bar
# We extract method/path from that line.

pattern='//encore:api public'
if [[ $INCLUDE_PRIVATE -eq 1 ]]; then
  pattern='//encore:api '
fi

# shellcheck disable=SC2016
endpoints=$(grep -R --line-number --include='*.go' "$pattern" "$PROJECT_ROOT" \
  | sed -nE 's@^.*//encore:api (public|private).*method=([^ ]+).*path=([^ ]+).*@\2 \3@p' \
  | sort -u)

if [[ "$OUTPUT_FORMAT" == "text" ]]; then
  echo "$endpoints"
  exit 0
fi

# JSON output (no jq dependency)
# Produce: [{"method":"GET","path":"/x"}, ...]

printf '['
first=1
while IFS= read -r line; do
  [[ -z "$line" ]] && continue
  method=${line%% *}
  path=${line#* }

  if [[ $first -eq 0 ]]; then
    printf ','
  fi
  first=0

  # Minimal JSON escaping for the path (paths should not contain quotes)
  printf '{"method":"%s","path":"%s"}' "$method" "$path"
done <<< "$endpoints"
printf ']\n'