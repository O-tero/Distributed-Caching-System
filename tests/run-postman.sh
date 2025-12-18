#!/usr/bin/env bash
# Run Postman collection using newman (optional)
#
# Usage:
#   ./tests/run-postman.sh
#   BASE_URL=http://localhost:9400 AUTH_TOKEN=... ./tests/run-postman.sh

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COLLECTION="$ROOT_DIR/tests/collections/postman/distributed-cache-system.postman_collection.json"
ENV_FILE="$ROOT_DIR/tests/collections/postman/distributed-cache-system.postman_environment.json"

BASE_URL="${BASE_URL:-}"
AUTH_TOKEN="${AUTH_TOKEN:-${API_TOKEN_ADMIN:-}}"

if ! command -v newman >/dev/null 2>&1; then
  echo "newman is not installed. Install with: npm install -g newman" >&2
  exit 1
fi

# Allow overriding via environment variables (Newman can override vars via --env-var)
args=(run "$COLLECTION" -e "$ENV_FILE")
if [[ -n "$BASE_URL" ]]; then
  args+=(--env-var "baseUrl=$BASE_URL")
fi
if [[ -n "$AUTH_TOKEN" ]]; then
  args+=(--env-var "authToken=$AUTH_TOKEN")
fi

newman "${args[@]}"
