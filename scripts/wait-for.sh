#!/usr/bin/env bash
# wait-for.sh - Wait for TCP port or HTTP endpoint to be available
#
# Purpose: Robust service readiness check for startup orchestration
#
# Usage:
#   ./wait-for.sh host:port --timeout 60
#   ./wait-for.sh http://host:port/health --timeout 60 --interval 2
#
# Examples:
#   ./wait-for.sh localhost:5432 --timeout 30
#   ./wait-for.sh http://localhost:9400/health --timeout 60
#   ./wait-for.sh postgres:5432 && echo "Postgres ready"
#
# Exit codes:
#   0 - Service is available
#   1 - Timeout or error
#
# Features:
#   - TCP port checking
#   - HTTP endpoint checking (200-399 response)
#   - Configurable timeout and retry interval
#   - Clear progress messages
#   - Works with Docker Compose service names

set -euo pipefail

# ============================================================================
# CONFIGURATION
# ============================================================================

TIMEOUT=60
INTERVAL=1
QUIET=0
HOST=""
PORT=""
HTTP_URL=""

# ============================================================================
# FUNCTIONS
# ============================================================================

log() {
    if [[ $QUIET -eq 0 ]]; then
        echo "[$(date +'%Y-%m-%d %H:%M:%S')] $*" >&2
    fi
}

error() {
    echo "[ERROR] $*" >&2
}

usage() {
    cat >&2 <<EOF
Usage: $0 <target> [options]

Wait for a service to become available.

Targets:
    host:port           Wait for TCP port
    http://...          Wait for HTTP endpoint (200-399 response)

Options:
    --timeout SECONDS   Maximum time to wait (default: 60)
    --interval SECONDS  Time between checks (default: 1)
    --quiet, -q         Suppress progress messages
    --help, -h          Show this help

Examples:
    $0 localhost:5432 --timeout 30
    $0 postgres:5432
    $0 http://localhost:9400/health --timeout 60
    $0 redis:6379 && echo "Redis ready"

Exit codes:
    0   Service available
    1   Timeout or error
EOF
    exit 1
}

# Check if TCP port is open
check_tcp() {
    local host=$1
    local port=$2
    
    # Try multiple methods for maximum compatibility
    
    # Method 1: nc (netcat) - most reliable
    if command -v nc >/dev/null 2>&1; then
        nc -z -w 1 "$host" "$port" >/dev/null 2>&1
        return $?
    fi
    
    # Method 2: timeout + bash pseudo-device
    if command -v timeout >/dev/null 2>&1; then
        timeout 1 bash -c "cat < /dev/null > /dev/tcp/$host/$port" >/dev/null 2>&1
        return $?
    fi
    
    # Method 3: telnet fallback
    if command -v telnet >/dev/null 2>&1; then
        (echo > "/dev/tcp/$host/$port") >/dev/null 2>&1
        return $?
    fi
    
    error "No suitable tool found for TCP checks (nc, timeout, or telnet required)"
    return 1
}

# Check if HTTP endpoint returns success
check_http() {
    local url=$1
    local status_code
    
    # Try curl first (most common)
    if command -v curl >/dev/null 2>&1; then
        status_code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 5 "$url" 2>/dev/null || echo "000")
        
        # Accept 200-399 as success
        if [[ $status_code -ge 200 && $status_code -lt 400 ]]; then
            return 0
        fi
        return 1
    fi
    
    # Try wget as fallback
    if command -v wget >/dev/null 2>&1; then
        wget --spider --timeout=5 -q "$url" >/dev/null 2>&1
        return $?
    fi
    
    error "No suitable tool found for HTTP checks (curl or wget required)"
    return 1
}

# Parse target (host:port or http://...)
parse_target() {
    local target=$1
    
    # HTTP(S) URL
    if [[ $target =~ ^https?:// ]]; then
        HTTP_URL=$target
        log "Waiting for HTTP endpoint: $HTTP_URL"
        return 0
    fi
    
    # host:port format
    if [[ $target =~ ^([^:]+):([0-9]+)$ ]]; then
        HOST="${BASH_REMATCH[1]}"
        PORT="${BASH_REMATCH[2]}"
        log "Waiting for TCP $HOST:$PORT"
        return 0
    fi
    
    error "Invalid target format: $target"
    error "Expected: host:port or http://..."
    return 1
}

# ============================================================================
# MAIN
# ============================================================================

# Parse arguments
if [[ $# -eq 0 ]]; then
    usage
fi

TARGET=$1
shift

while [[ $# -gt 0 ]]; do
    case $1 in
        --timeout)
            TIMEOUT=$2
            shift 2
            ;;
        --interval)
            INTERVAL=$2
            shift 2
            ;;
        --quiet|-q)
            QUIET=1
            shift
            ;;
        --help|-h)
            usage
            ;;
        --)
            shift
            break
            ;;
        *)
            error "Unknown option: $1"
            usage
            ;;
    esac
done

# Parse target
if ! parse_target "$TARGET"; then
    exit 1
fi

# Start waiting
START_TIME=$(date +%s)
ELAPSED=0

log "Timeout: ${TIMEOUT}s, Check interval: ${INTERVAL}s"

while [[ $ELAPSED -lt $TIMEOUT ]]; do
    # Check availability
    if [[ -n $HTTP_URL ]]; then
        if check_http "$HTTP_URL"; then
            log "✓ HTTP endpoint available: $HTTP_URL"
            exit 0
        fi
    else
        if check_tcp "$HOST" "$PORT"; then
            log "✓ TCP port available: $HOST:$PORT"
            exit 0
        fi
    fi
    
    # Not ready yet
    if [[ $QUIET -eq 0 ]]; then
        echo -n "." >&2
    fi
    
    sleep "$INTERVAL"
    
    CURRENT_TIME=$(date +%s)
    ELAPSED=$((CURRENT_TIME - START_TIME))
done

# Timeout reached
echo "" >&2
if [[ -n $HTTP_URL ]]; then
    error "Timeout waiting for $HTTP_URL (${TIMEOUT}s elapsed)"
else
    error "Timeout waiting for $HOST:$PORT (${TIMEOUT}s elapsed)"
fi

exit 1