#!/usr/bin/env bash
# Comprehensive API Endpoint Testing Script
#
# Tests all Encore public API endpoints across all services with detailed reporting.
#
# Usage:
#   ./tests/test-all-endpoints.sh
#   ./tests/test-all-endpoints.sh --verbose
#   ./tests/test-all-endpoints.sh --service cache-manager
#   ./tests/test-all-endpoints.sh --base-url http://localhost:4000
#
# Requirements:
#   - curl
# Optional:
#   - jq (enables JSON assertions)
#
# NOTE:
# - In this repository, the Encore *dashboard* is commonly available on port 9400,
#   while the *API gateway* is commonly available on port 4000.
# - Use --base-url / APP_URL / BASE_URL to point tests at the API gateway.
# - You can override per-service URLs if you deploy services separately.

set -euo pipefail

# ============================================================================
# CONFIGURATION
# ============================================================================

# Base URL for API requests.
#
# In this repo, Encore's developer dashboard can be on ENCORE_PORT (default 9400)
# while the API gateway is often on ENCORE_API_PORT (default 4000).
API_PORT="${ENCORE_API_PORT:-4000}"
APP_URL="${APP_URL:-${BASE_URL:-http://localhost:$API_PORT}}"
DEFAULT_APP_URL="$APP_URL"

# Optional: Encore dashboard URL (not used for API calls, just displayed)
DASHBOARD_URL="${DASHBOARD_URL:-http://localhost:${ENCORE_PORT:-9400}}"

# Service URLs (default to APP_URL unless explicitly overridden)
CACHE_MANAGER_URL="${CACHE_MANAGER_URL:-$APP_URL}"
INVALIDATION_URL="${INVALIDATION_URL:-$APP_URL}"
WARMING_URL="${WARMING_URL:-$APP_URL}"
MONITORING_URL="${MONITORING_URL:-$APP_URL}"

# Auth token (if required)
AUTH_TOKEN="${API_TOKEN_ADMIN:-}"

# Test configuration
VERBOSE=0
TEST_SERVICE="all"
FAILED_TESTS=0
PASSED_TESTS=0
TOTAL_TESTS=0

# Request timeouts
CONNECT_TIMEOUT_SECONDS="${CONNECT_TIMEOUT_SECONDS:-2}"
MAX_TIME_SECONDS="${MAX_TIME_SECONDS:-10}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# last response (for debugging/assertions)
LAST_STATUS=""
LAST_BODY=""

# ============================================================================
# HELPER FUNCTIONS
# ============================================================================

has_cmd() {
    command -v "$1" >/dev/null 2>&1
}

log() {
    echo -e "${GREEN}[INFO]${NC} $*"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $*" >&2
}

error() {
    echo -e "${RED}[ERROR]${NC} $*" >&2
}

success() {
    echo -e "${GREEN}✓${NC} $*"
}

fail() {
    echo -e "${RED}✗${NC} $*"
}

verbose() {
    if [[ $VERBOSE -eq 1 ]]; then
        echo -e "${BLUE}[DEBUG]${NC} $*"
    fi
}

require_curl() {
    if ! has_cmd curl; then
        error "curl is required but not installed"
        exit 1
    fi
}

jq_available() {
    has_cmd jq
}

# RFC3339 timestamp in UTC
now_rfc3339() {
    date -u +"%Y-%m-%dT%H:%M:%SZ"
}

minutes_ago_rfc3339() {
    local minutes="$1"
    date -u -d "${minutes} minutes ago" +"%Y-%m-%dT%H:%M:%SZ"
}

# Make HTTP request and return body + newline + status
http_request() {
    local method=$1
    local url=$2
    local data=${3:-}

    local headers=(-H "Content-Type: application/json")
    if [[ -n $AUTH_TOKEN ]]; then
        headers+=(-H "Authorization: Bearer $AUTH_TOKEN")
    fi

    local curl_args=(
        -sS
        --connect-timeout "$CONNECT_TIMEOUT_SECONDS"
        --max-time "$MAX_TIME_SECONDS"
        -w "\n%{http_code}"
        -X "$method"
        "$url"
        "${headers[@]}"
    )

    if [[ -n $data ]]; then
        curl "${curl_args[@]}" -d "$data"
    else
        curl "${curl_args[@]}"
    fi
}

# Internal: check if status is in allowed list "200" or "200,204" etc.
status_in_allowed_list() {
    local status="$1"
    local allowed_csv="$2"

    IFS=',' read -r -a allowed <<< "$allowed_csv"
    for a in "${allowed[@]}"; do
        if [[ "$status" == "$a" ]]; then
            return 0
        fi
    done
    return 1
}

# Assert helpers (requires jq)
assert_json() {
    local jq_expr="$1"
    local expected="$2"

    if ! jq_available; then
        warn "jq not installed; skipping JSON assertion: $jq_expr == $expected"
        return 0
    fi

    local actual
    actual=$(echo "$LAST_BODY" | jq -r "$jq_expr" 2>/dev/null || echo "__jq_error__")

    if [[ "$actual" == "__jq_error__" ]]; then
        return 1
    fi

    if [[ "$expected" == "__present__" ]]; then
        [[ -n "$actual" && "$actual" != "null" ]]
        return $?
    fi

    if [[ "$expected" =~ ^re: ]]; then
        local re="${expected#re:}"
        [[ "$actual" =~ $re ]]
        return $?
    fi

    [[ "$actual" == "$expected" ]]
}

# Test endpoint and report result.
#
# Signature:
#   test_endpoint "name" METHOD URL DATA EXPECTED_CODES [JQ_EXPR EXPECTED]...
#
# EXPECTED_CODES can be a comma-separated list (e.g., "200,204").
# If jq is installed, additional assertions are evaluated as pairs.

test_endpoint() {
    local test_name=$1
    local method=$2
    local url=$3
    local data=${4:-}
    local expected_codes=${5:-200}
    shift 5 || true

    # IMPORTANT: with `set -e`, arithmetic post-increment can terminate the script.
    # Use pre-increment so the expression returns a non-zero value.
    ((++TOTAL_TESTS))

    verbose "Testing: $test_name"
    verbose "  Method: $method"
    verbose "  URL: $url"
    [[ -n $data ]] && verbose "  Data: $data"

    local response
    if ! response=$(http_request "$method" "$url" "$data" 2>&1); then
        fail "$test_name - Request failed"
        verbose "  Error: $response"
        ((++FAILED_TESTS))
        return 1
    fi

    LAST_STATUS=$(echo "$response" | tail -n 1)
    LAST_BODY=$(echo "$response" | sed '$d')

    verbose "  Status: $LAST_STATUS"
    [[ -n "$LAST_BODY" ]] && verbose "  Body: ${LAST_BODY:0:200}..."

    if ! status_in_allowed_list "$LAST_STATUS" "$expected_codes"; then
        fail "$test_name - Expected [$expected_codes], got $LAST_STATUS"
        ((++FAILED_TESTS))
        return 1
    fi

    # Optional JSON assertions
    local ok=1
    if [[ $# -gt 0 ]]; then
        if (( $# % 2 != 0 )); then
            fail "$test_name - Internal error: assertions must be passed in pairs"
            ((++FAILED_TESTS))
            return 1
        fi

        while [[ $# -gt 0 ]]; do
            local jq_expr="$1"
            local expected="$2"
            shift 2

            if ! assert_json "$jq_expr" "$expected"; then
                ok=0
                verbose "  Assertion failed: $jq_expr == $expected"
            fi
        done
    fi

    if [[ $ok -eq 1 ]]; then
        success "$test_name - HTTP $LAST_STATUS"
        ((++PASSED_TESTS))
        return 0
    else
        fail "$test_name - HTTP $LAST_STATUS (assertion failed)"
        ((++FAILED_TESTS))
        return 1
    fi
}

# ============================================================================
# TEST SUITES
# ============================================================================

test_cache_manager() {
    log "Testing cache-manager endpoints"
    echo ""

    # Health check (Encore dashboard). The API gateway port may not expose /health.
    test_endpoint \
        "GET /health" \
        "GET" \
        "$DASHBOARD_URL/health" \
        "" \
        "200,404"

    # PUT /api/cache/entry/:key
    test_endpoint \
        "PUT /api/cache/entry/:key - Store value" \
        "PUT" \
        "$CACHE_MANAGER_URL/api/cache/entry/test:user:123" \
        '{"value":{"name":"John Doe","email":"john@example.com","age":30},"ttl":60}' \
        "200" \
        '.success' 'true' \
        '.expires_at' '__present__'

    # GET /api/cache/entry/:key
    test_endpoint \
        "GET /api/cache/entry/:key - Retrieve value" \
        "GET" \
        "$CACHE_MANAGER_URL/api/cache/entry/test:user:123" \
        "" \
        "200" \
        '.hit' 'true' \
        '.source' 'l1' \
        '.value' '__present__'

    # GET miss (with default codebase config, this typically errors because origin fetcher is not configured)
    test_endpoint \
        "GET /api/cache/entry/:key - Cache miss (expected error)" \
        "GET" \
        "$CACHE_MANAGER_URL/api/cache/entry/test:missing:key" \
        "" \
        "400,404,500"

    # POST /api/cache/invalidate
    test_endpoint \
        "POST /api/cache/invalidate - Invalidate by keys" \
        "POST" \
        "$CACHE_MANAGER_URL/api/cache/invalidate" \
        '{"keys":["test:user:123"],"pattern":""}' \
        "200" \
        '.success' 'true' \
        '.invalidated' '1'

    # GET /api/cache/metrics
    test_endpoint \
        "GET /api/cache/metrics" \
        "GET" \
        "$CACHE_MANAGER_URL/api/cache/metrics" \
        "" \
        "200" \
        '.hits' '__present__' \
        '.misses' '__present__' \
        '.l1_size' '__present__'

    echo ""
}

test_invalidation() {
    log "Testing invalidation endpoints"
    echo ""

    # POST /invalidate/key
    test_endpoint \
        "POST /invalidate/key" \
        "POST" \
        "$INVALIDATION_URL/invalidate/key" \
        '{"keys":["test:inv:user:1"],"triggered_by":"tests","request_id":""}' \
        "200" \
        '.success' 'true' \
        '.invalidated_count' '1' \
        '.request_id' '__present__'

    # POST /invalidate/key - invalid payload (empty keys)
    test_endpoint \
        "POST /invalidate/key - Empty keys (expected error)" \
        "POST" \
        "$INVALIDATION_URL/invalidate/key" \
        '{"keys":[],"triggered_by":"tests"}' \
        "400,500"

    # POST /invalidate/pattern
    test_endpoint \
        "POST /invalidate/pattern" \
        "POST" \
        "$INVALIDATION_URL/invalidate/pattern" \
        '{"pattern":"test:inv:*","triggered_by":"tests","request_id":""}' \
        "200" \
        '.success' 'true' \
        '.pattern' 'test:inv:*' \
        '.request_id' '__present__'

    # POST /invalidate/pattern - invalid payload (empty pattern)
    test_endpoint \
        "POST /invalidate/pattern - Empty pattern (expected error)" \
        "POST" \
        "$INVALIDATION_URL/invalidate/pattern" \
        '{"pattern":"","triggered_by":"tests"}' \
        "400,500"

    # GET /audit/logs
    test_endpoint \
        "GET /audit/logs" \
        "GET" \
        "$INVALIDATION_URL/audit/logs?limit=10&offset=0" \
        "" \
        "200" \
        '.logs' '__present__' \
        '.total_count' '__present__'

    # GET /invalidate/metrics
    test_endpoint \
        "GET /invalidate/metrics" \
        "GET" \
        "$INVALIDATION_URL/invalidate/metrics" \
        "" \
        "200" \
        '.total_invalidations' '__present__' \
        '.errors' '__present__'

    echo ""
}

test_warming() {
    log "Testing warming endpoints"
    echo ""

    # POST /warm/key
    test_endpoint \
        "POST /warm/key" \
        "POST" \
        "$WARMING_URL/warm/key" \
        '{"keys":["warm:test:key:1","warm:test:key:2"],"priority":50,"strategy":"priority"}' \
        "200" \
        '.success' 'true' \
        '.queued' '__present__' \
        '.job_id' '__present__'

    # POST /warm/key - invalid payload
    test_endpoint \
        "POST /warm/key - Empty keys (expected error)" \
        "POST" \
        "$WARMING_URL/warm/key" \
        '{"keys":[]}' \
        "400,500"

    # POST /warm/pattern
    test_endpoint \
        "POST /warm/pattern" \
        "POST" \
        "$WARMING_URL/warm/pattern" \
        '{"pattern":"warm:test:*","limit":10,"priority":50,"strategy":"priority"}' \
        "200" \
        '.success' 'true' \
        '.pattern' 'warm:test:*' \
        '.job_id' '__present__'

    # GET /warm/status
    test_endpoint \
        "GET /warm/status" \
        "GET" \
        "$WARMING_URL/warm/status" \
        "" \
        "200" \
        '.active_jobs' '__present__' \
        '.queued_tasks' '__present__' \
        '.metrics.jobs_total' '__present__'

    # POST /warm/trigger-predictive
    test_endpoint \
        "POST /warm/trigger-predictive" \
        "POST" \
        "$WARMING_URL/warm/trigger-predictive" \
        "" \
        "200" \
        '.success' '__present__'

    # GET /warm/config
    test_endpoint \
        "GET /warm/config" \
        "GET" \
        "$WARMING_URL/warm/config" \
        "" \
        "200" \
        '.config.max_origin_rps' '__present__' \
        '.config.default_strategy' '__present__'

    # POST /warm/config
    test_endpoint \
        "POST /warm/config" \
        "POST" \
        "$WARMING_URL/warm/config" \
        '{"max_origin_rps":200}' \
        "200" \
        '.config.max_origin_rps' '200'

    echo ""
}

test_monitoring() {
    log "Testing monitoring endpoints"
    echo ""

    # GET /monitoring/metrics
    test_endpoint \
        "GET /monitoring/metrics" \
        "GET" \
        "$MONITORING_URL/monitoring/metrics?window=1m" \
        "" \
        "200" \
        '.window' '__present__' \
        '.hit_rate' '__present__'

    # POST /monitoring/aggregated
    local start_time
    local end_time
    start_time=$(minutes_ago_rfc3339 10)
    end_time=$(now_rfc3339)

    test_endpoint \
        "POST /monitoring/aggregated" \
        "POST" \
        "$MONITORING_URL/monitoring/aggregated" \
        "{\"start_time\":\"$start_time\",\"end_time\":\"$end_time\",\"interval\":\"1m\"}" \
        "200" \
        '.data_points' '__present__' \
        '.summary.window' '__present__'

    # GET /monitoring/alerts
    test_endpoint \
        "GET /monitoring/alerts" \
        "GET" \
        "$MONITORING_URL/monitoring/alerts" \
        "" \
        "200" \
        '.active_alerts' '__present__' \
        '.alert_stats.active_count' '__present__'

    echo ""
}

test_monitoring_dashboard() {
    log "Testing monitoring dashboard endpoints"
    echo ""

    # POST /monitoring/dashboard/overview
    test_endpoint \
        "POST /monitoring/dashboard/overview" \
        "POST" \
        "$MONITORING_URL/monitoring/dashboard/overview" \
        '{"time_range":"1h"}' \
        "200" \
        '.summary.total_requests' '__present__' \
        '.system_health.status' '__present__'

    # POST /monitoring/dashboard/latency-distribution
    test_endpoint \
        "POST /monitoring/dashboard/latency-distribution" \
        "POST" \
        "$MONITORING_URL/monitoring/dashboard/latency-distribution" \
        '{"window":"5m"}' \
        "200" \
        '.buckets' '__present__'

    # POST /monitoring/dashboard/heatmap
    local start_time
    local end_time
    start_time=$(minutes_ago_rfc3339 60)
    end_time=$(now_rfc3339)

    test_endpoint \
        "POST /monitoring/dashboard/heatmap" \
        "POST" \
        "$MONITORING_URL/monitoring/dashboard/heatmap" \
        "{\"start_time\":\"$start_time\",\"end_time\":\"$end_time\",\"metric\":\"hit_rate\"}" \
        "200" \
        '.x_labels' '__present__' \
        '.y_labels' '__present__' \
        '.data' '__present__'

    # POST /monitoring/dashboard/comparison
    local p1s p1e p2s p2e
    p1s=$(minutes_ago_rfc3339 120)
    p1e=$(minutes_ago_rfc3339 60)
    p2s=$(minutes_ago_rfc3339 60)
    p2e=$(now_rfc3339)

    test_endpoint \
        "POST /monitoring/dashboard/comparison" \
        "POST" \
        "$MONITORING_URL/monitoring/dashboard/comparison" \
        "{\"period1_start\":\"$p1s\",\"period1_end\":\"$p1e\",\"period2_start\":\"$p2s\",\"period2_end\":\"$p2e\"}" \
        "200" \
        '.differences' '__present__'

    # GET /monitoring/dashboard/stream
    test_endpoint \
        "GET /monitoring/dashboard/stream" \
        "GET" \
        "$MONITORING_URL/monitoring/dashboard/stream" \
        "" \
        "200" \
        '.id' '__present__' \
        '.created_at' '__present__'

    # POST /monitoring/dashboard/export
    test_endpoint \
        "POST /monitoring/dashboard/export" \
        "POST" \
        "$MONITORING_URL/monitoring/dashboard/export" \
        "{\"start_time\":\"$start_time\",\"end_time\":\"$end_time\",\"format\":\"json\",\"metrics\":[\"cache_hits\",\"cache_misses\",\"hit_rate\"]}" \
        "200" \
        '.format' 'json' \
        '.data' '__present__' \
        '.filename' '__present__'

    echo ""
}

test_integration() {
    log "Running end-to-end integration scenario"
    echo ""

    # 1) Store a cache entry
    test_endpoint \
        "Integration: PUT cache entry" \
        "PUT" \
        "$CACHE_MANAGER_URL/api/cache/entry/int:user:alice" \
        '{"value":{"name":"Alice","role":"admin"},"ttl":60}' \
        "200" \
        '.success' 'true'

    # 2) Read it back
    test_endpoint \
        "Integration: GET cache entry" \
        "GET" \
        "$CACHE_MANAGER_URL/api/cache/entry/int:user:alice" \
        "" \
        "200" \
        '.hit' 'true'

    # 3) Invalidate it via cache-manager
    test_endpoint \
        "Integration: POST cache invalidate" \
        "POST" \
        "$CACHE_MANAGER_URL/api/cache/invalidate" \
        '{"keys":["int:user:alice"]}' \
        "200" \
        '.invalidated' '1'

    # 4) Observe that a subsequent read returns an error (miss)
    test_endpoint \
        "Integration: GET after invalidation (expected error)" \
        "GET" \
        "$CACHE_MANAGER_URL/api/cache/entry/int:user:alice" \
        "" \
        "400,404,500"

    # 5) Trigger warming
    test_endpoint \
        "Integration: POST warm/pattern" \
        "POST" \
        "$WARMING_URL/warm/pattern" \
        '{"pattern":"int:*","limit":10,"priority":80,"strategy":"priority"}' \
        "200" \
        '.success' 'true'

    # 6) Fetch monitoring metrics
    test_endpoint \
        "Integration: GET monitoring metrics" \
        "GET" \
        "$MONITORING_URL/monitoring/metrics?window=1m" \
        "" \
        "200"

    echo ""
}

# ============================================================================
# MAIN EXECUTION
# ============================================================================

usage() {
    cat <<EOF
Usage: $0 [options]

Test all API endpoints across all services.

Options:
    --verbose, -v           Verbose output (show request/response details)
    --base-url URL          Base URL for all API requests (default: http://localhost:${ENCORE_API_PORT:-4000})
    --service SERVICE       Test specific suite (cache-manager, invalidation, warming, monitoring, dashboard, integration, all)
    --help, -h              Show this help

Examples:
    $0                                   # Test all suites
    $0 --verbose                         # Test with detailed output
    $0 --base-url http://localhost:4000  # Test against the Encore API gateway
    $0 --service cache-manager           # Test only cache-manager endpoints
    $0 --service dashboard -v            # Test monitoring dashboard endpoints verbosely

Environment Variables:
    ENCORE_API_PORT        API gateway port (default: 4000)
    ENCORE_PORT            Dashboard port (default: 9400)
    APP_URL                API base URL (default: http://localhost:${ENCORE_API_PORT:-4000})
    BASE_URL               Alias for APP_URL
    DASHBOARD_URL          Optional dashboard base URL (default: http://localhost:${ENCORE_PORT:-9400})
    CACHE_MANAGER_URL      Override cache-manager base URL (default: APP_URL)
    INVALIDATION_URL       Override invalidation base URL (default: APP_URL)
    WARMING_URL            Override warming base URL (default: APP_URL)
    MONITORING_URL         Override monitoring base URL (default: APP_URL)
    API_TOKEN_ADMIN        Admin API token (for auth)
    CONNECT_TIMEOUT_SECONDS  curl connect timeout (default: 2)
    MAX_TIME_SECONDS         curl max time (default: 10)

EOF
    exit 0
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --verbose|-v)
            VERBOSE=1
            shift
            ;;
        --base-url)
            APP_URL=$2
            # If the service URLs were not explicitly overridden (they still match the
            # initial default APP_URL), point them at the new base URL.
            if [[ "$CACHE_MANAGER_URL" == "$DEFAULT_APP_URL" ]]; then CACHE_MANAGER_URL=$APP_URL; fi
            if [[ "$INVALIDATION_URL" == "$DEFAULT_APP_URL" ]]; then INVALIDATION_URL=$APP_URL; fi
            if [[ "$WARMING_URL" == "$DEFAULT_APP_URL" ]]; then WARMING_URL=$APP_URL; fi
            if [[ "$MONITORING_URL" == "$DEFAULT_APP_URL" ]]; then MONITORING_URL=$APP_URL; fi
            shift 2
            ;;
        --service)
            TEST_SERVICE=$2
            shift 2
            ;;
        --help|-h)
            usage
            ;;
        *)
            error "Unknown option: $1"
            usage
            ;;
    esac
done

# Print header
echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  Distributed Cache System - API Endpoint Test Suite"
echo "════════════════════════════════════════════════════════════════"
echo ""
require_curl

# Basic sanity: make sure we are hitting the API gateway (JSON endpoints), not the Encore dashboard.
check_json_endpoint() {
    local name="$1"
    local url="$2"

    local headers
    headers=$(curl -sS -D - -o /dev/null --connect-timeout "$CONNECT_TIMEOUT_SECONDS" --max-time "$MAX_TIME_SECONDS" "$url" 2>/dev/null || true)

    local status
    status=$(echo "$headers" | awk 'NR==1 {print $2}')
    local ctype
    ctype=$(echo "$headers" | awk -F': ' 'tolower($1)=="content-type" {print tolower($2)}' | head -n 1 | tr -d '\r')

    if [[ "$status" != "200" ]]; then
        error "$name is not reachable at $url (HTTP $status)"
        return 1
    fi

    if [[ "$ctype" != application/json* ]]; then
        error "$name did not return JSON at $url (content-type=$ctype)"
        error "This usually means you're pointing at the Encore dashboard (port 9400) instead of the API gateway (often port 4000)."
        error "Try: ./tests/test-all-endpoints.sh --base-url http://localhost:4000"
        return 1
    fi

    success "$name reachable"
    return 0
}

log "Starting comprehensive endpoint testing..."
log "Target base URLs:"
echo "  - API (APP_URL):  $APP_URL"
echo "  - Dashboard:      $DASHBOARD_URL"
echo "  - Cache Manager:  $CACHE_MANAGER_URL"
echo "  - Invalidation:   $INVALIDATION_URL"
echo "  - Warming:        $WARMING_URL"
echo "  - Monitoring:     $MONITORING_URL"
echo ""

# Check if services are running (and that we're hitting the API gateway)
log "Checking service availability..."
check_json_endpoint "cache-manager" "$CACHE_MANAGER_URL/api/cache/metrics" || exit 1
check_json_endpoint "invalidation" "$INVALIDATION_URL/invalidate/metrics" || exit 1
check_json_endpoint "warming" "$WARMING_URL/warm/status" || exit 1
check_json_endpoint "monitoring" "$MONITORING_URL/monitoring/metrics?window=1m" || exit 1
success "All API endpoints are reachable"
echo ""

# Run tests based on selection
case $TEST_SERVICE in
    cache-manager)
        test_cache_manager
        ;;
    invalidation)
        test_invalidation
        ;;
    warming)
        test_warming
        ;;
    monitoring)
        test_monitoring
        ;;
    dashboard)
        test_monitoring_dashboard
        ;;
    integration)
        test_integration
        ;;
    all)
        test_cache_manager
        test_invalidation
        test_warming
        test_monitoring
        test_monitoring_dashboard
        test_integration
        ;;
    *)
        error "Invalid service: $TEST_SERVICE"
        exit 1
        ;;
esac

# Print summary
echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  Test Summary"
echo "════════════════════════════════════════════════════════════════"
echo ""
echo "Total Tests:  $TOTAL_TESTS"
echo -e "Passed:       ${GREEN}$PASSED_TESTS${NC}"
echo -e "Failed:       ${RED}$FAILED_TESTS${NC}"
echo ""

if [[ $FAILED_TESTS -eq 0 ]]; then
    echo -e "${GREEN}✓ All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}✗ Some tests failed${NC}"
    exit 1
fi