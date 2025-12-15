#!/usr/bin/env bash
# load_test.sh - Performance and load testing for cache system
#
# Purpose: Generate realistic load patterns and measure performance
#
# Usage:
#   ./scripts/load_test.sh                                    # Default: curl mode, 100 req/s, 30s
#   ./scripts/load_test.sh --mode vegeta --rate 500           # Vegeta mode
#   ./scripts/load_test.sh --pattern hotspot --duration 60s   # Hotspot pattern
#
# Modes:
#   vegeta  - High-performance load testing (requires vegeta)
#   curl    - Simple curl-based testing (fallback)
#
# Patterns:
#   uniform  - Even distribution across keys
#   hotspot  - 80% of requests to 20% of keys (realistic)
#   burst    - Sudden traffic spikes

set -euo pipefail

# ============================================================================
# CONFIGURATION
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Default settings
MODE="auto"  # auto, vegeta, curl
RATE=100
DURATION="30s"
PATTERN="uniform"  # uniform, hotspot, burst
CACHE_URL="${CACHE_MANAGER_URL:-http://localhost:9400}"
FORCE=0
WORKERS=10

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m'

# ============================================================================
# FUNCTIONS
# ============================================================================

log() {
    echo -e "${GREEN}[$(date +'%H:%M:%S')]${NC} $*"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $*" >&2
}

error() {
    echo -e "${RED}[ERROR]${NC} $*" >&2
}

usage() {
    cat <<EOF
Usage: $0 [options]

Load testing tool for distributed caching system.

Options:
    --mode MODE            Testing mode: auto, vegeta, curl (default: auto)
    --rate N               Requests per second (default: 100)
    --duration DURATION    Test duration (default: 30s)
    --pattern PATTERN      Load pattern: uniform, hotspot, burst (default: uniform)
    --cache-url URL        Cache manager URL (default: $CACHE_URL)
    --workers N            Number of parallel workers for curl mode (default: 10)
    --force                Allow high rate tests (>500 req/s)
    --help, -h             Show this help

Modes:
    auto      Automatically choose best available tool
    vegeta    Use vegeta for high-performance testing
    curl      Use curl-based parallel testing (fallback)

Patterns:
    uniform   Even distribution across all keys
    hotspot   80% requests to 20% of keys (Pareto distribution)
    burst     Sudden traffic spikes with pauses

Examples:
    $0                                        # Auto mode, 100 req/s, 30s
    $0 --mode vegeta --rate 500 --duration 60s
    $0 --pattern hotspot --rate 300
    $0 --mode curl --workers 20 --rate 200

Safety:
    High-rate tests (>500 req/s) require --force flag to prevent accidents.
    DO NOT run load tests against shared/production environments.

Install vegeta (recommended):
    Ubuntu/Debian:
      wget https://github.com/tsenart/vegeta/releases/download/v12.11.0/vegeta_12.11.0_linux_amd64.tar.gz
      tar xzf vegeta_12.11.0_linux_amd64.tar.gz
      sudo mv vegeta /usr/local/bin/

EOF
    exit 0
}

check_safety() {
    # Prevent accidental high-rate tests
    if [[ $RATE -gt 500 && $FORCE -eq 0 ]]; then
        error "High rate ($RATE req/s) requires --force flag for safety"
        error "This prevents accidental DoS on local services"
        exit 1
    fi
    
    # Warn about localhost testing
    if [[ ! $CACHE_URL =~ localhost|127\.0\.0\.1 ]]; then
        warn "Testing against non-localhost URL: $CACHE_URL"
        read -p "Continue? (y/N) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            exit 1
        fi
    fi
}

detect_mode() {
    if [[ $MODE == "auto" ]]; then
        if command -v vegeta >/dev/null 2>&1; then
            MODE="vegeta"
            log "Using vegeta (detected)"
        else
            MODE="curl"
            log "Using curl mode (vegeta not found)"
            warn "Install vegeta for better performance: https://github.com/tsenart/vegeta"
        fi
    fi
    
    # Validate mode
    case $MODE in
        vegeta)
            if ! command -v vegeta >/dev/null 2>&1; then
                error "vegeta not found. Install or use --mode curl"
                exit 1
            fi
            ;;
        curl)
            if ! command -v curl >/dev/null 2>&1; then
                error "curl not found. Install: sudo apt-get install curl"
                exit 1
            fi
            ;;
        *)
            error "Invalid mode: $MODE"
            exit 1
            ;;
    esac
}

generate_keys_uniform() {
    # Generate 1000 keys evenly
    for i in $(seq 1 1000); do
        echo "users:$i"
    done
}

generate_keys_hotspot() {
    # 80% requests to top 20% keys (200 keys)
    # 20% requests to remaining 80% keys (800 keys)
    
    # Hot keys (80% of traffic)
    for i in $(seq 1 8); do
        for j in $(seq 1 200); do
            echo "users:$j"
        done
    done
    
    # Cold keys (20% of traffic)
    for i in $(seq 1 2); do
        for j in $(seq 201 1000); do
            echo "users:$j"
        done
    done
}

run_vegeta_test() {
    log "Running vegeta load test..."
    log "Rate: $RATE req/s, Duration: $DURATION, Pattern: $PATTERN"
    
    # Generate target file
    local targets_file="/tmp/vegeta_targets_$$.txt"
    
    case $PATTERN in
        uniform)
            keys=$(generate_keys_uniform)
            ;;
        hotspot)
            keys=$(generate_keys_hotspot)
            ;;
        burst)
            keys=$(generate_keys_uniform)
            ;;
    esac
    
    # Create vegeta targets file
    echo "$keys" | while read key; do
        echo "GET $CACHE_URL/api/cache/$key"
    done > "$targets_file"
    
    log "Generated $(wc -l < $targets_file) targets"
    log "Starting attack..."
    
    # Run vegeta attack
    local results_file="/tmp/vegeta_results_$$.bin"
    
    if [[ $PATTERN == "burst" ]]; then
        log "Burst pattern: 10s @ ${RATE}req/s, 5s pause, repeat"
        # TODO: Implement burst pattern with multiple attacks
        vegeta attack -targets="$targets_file" -rate="$RATE" -duration="$DURATION" > "$results_file"
    else
        vegeta attack -targets="$targets_file" -rate="$RATE" -duration="$DURATION" > "$results_file"
    fi
    
    log "Attack complete. Generating report..."
    
    # Generate reports
    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${GREEN}Load Test Results (Vegeta)${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    
    vegeta report < "$results_file"
    
    echo ""
    echo -e "${YELLOW}Latency Histogram:${NC}"
    vegeta report -type='hist[0,5ms,10ms,25ms,50ms,100ms,250ms,500ms,1s]' < "$results_file"
    
    # Cleanup
    rm -f "$targets_file" "$results_file"
}

run_curl_test() {
    log "Running curl-based load test..."
    log "Rate: $RATE req/s, Duration: $DURATION, Workers: $WORKERS, Pattern: $PATTERN"
    
    warn "Curl mode provides approximate testing. Use vegeta for accurate results."
    
    # Generate keys
    case $PATTERN in
        uniform)
            keys=($(generate_keys_uniform | head -n 100))
            ;;
        hotspot)
            keys=($(generate_keys_hotspot | head -n 100))
            ;;
        burst)
            keys=($(generate_keys_uniform | head -n 100))
            ;;
    esac
    
    # Parse duration
    local duration_sec
    if [[ $DURATION =~ ^([0-9]+)s$ ]]; then
        duration_sec=${BASH_REMATCH[1]}
    else
        duration_sec=30
    fi
    
    # Calculate requests per worker
    local total_requests=$((RATE * duration_sec))
    local requests_per_worker=$((total_requests / WORKERS))
    local delay=$(echo "scale=6; $WORKERS / $RATE" | bc)
    
    log "Total requests: $total_requests, Per worker: $requests_per_worker, Delay: ${delay}s"
    
    # Results file
    local results_file="/tmp/curl_results_$$.txt"
    > "$results_file"
    
    # Start workers
    local pids=()
    for worker in $(seq 1 $WORKERS); do
        (
            for i in $(seq 1 $requests_per_worker); do
                # Select random key
                local key_idx=$((RANDOM % ${#keys[@]}))
                local key=${keys[$key_idx]}
                
                # Make request and measure time
                local start=$(date +%s%N)
                local http_code=$(curl -s -o /dev/null -w "%{http_code}" \
                    --max-time 5 \
                    "$CACHE_URL/api/cache/$key" 2>/dev/null || echo "000")
                local end=$(date +%s%N)
                
                local latency_ms=$(( (end - start) / 1000000 ))
                
                echo "$http_code,$latency_ms" >> "$results_file"
                
                # Delay between requests
                sleep "$delay" 2>/dev/null || true
            done
        ) &
        pids+=($!)
    done
    
    log "Started $WORKERS workers (PIDs: ${pids[*]})"
    log "Test in progress..."
    
    # Wait for completion with progress
    local completed=0
    while [[ $completed -lt $WORKERS ]]; do
        completed=0
        for pid in "${pids[@]}"; do
            if ! kill -0 $pid 2>/dev/null; then
                ((completed++))
            fi
        done
        sleep 1
        printf "\rWorkers completed: %d/%d" "$completed" "$WORKERS"
    done
    echo ""
    
    log "Test complete. Analyzing results..."
    
    # Analyze results
    local total=$(wc -l < "$results_file")
    local success=$(grep -c "^2" "$results_file" || echo 0)
    local errors=$((total - success))
    local success_rate=$(echo "scale=2; $success * 100 / $total" | bc)
    
    # Calculate latency stats
    local latencies=$(awk -F, '{print $2}' "$results_file" | sort -n)
    local min=$(echo "$latencies" | head -n 1)
    local max=$(echo "$latencies" | tail -n 1)
    local p50=$(echo "$latencies" | awk "NR==$((total / 2))")
    local p95=$(echo "$latencies" | awk "NR==$((total * 95 / 100))")
    local p99=$(echo "$latencies" | awk "NR==$((total * 99 / 100))")
    
    # Print report
    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${GREEN}Load Test Results (Curl)${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    echo "Requests:      $total"
    echo "Success:       $success (${success_rate}%)"
    echo "Errors:        $errors"
    echo ""
    echo "Latency:"
    echo "  Min:         ${min}ms"
    echo "  P50:         ${p50}ms"
    echo "  P95:         ${p95}ms"
    echo "  P99:         ${p99}ms"
    echo "  Max:         ${max}ms"
    echo ""
    
    # Cleanup
    rm -f "$results_file"
}

# ============================================================================
# MAIN
# ============================================================================

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --mode)
            MODE=$2
            shift 2
            ;;
        --rate)
            RATE=$2
            shift 2
            ;;
        --duration)
            DURATION=$2
            shift 2
            ;;
        --pattern)
            PATTERN=$2
            shift 2
            ;;
        --cache-url)
            CACHE_URL=$2
            shift 2
            ;;
        --workers)
            WORKERS=$2
            shift 2
            ;;
        --force)
            FORCE=1
            shift
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

# Safety checks
check_safety

# Detect and validate mode
detect_mode

# Run test
log "Starting load test"
log "Target: $CACHE_URL"
log "Mode: $MODE, Rate: $RATE req/s, Duration: $DURATION, Pattern: $PATTERN"

case $MODE in
    vegeta)
        run_vegeta_test
        ;;
    curl)
        run_curl_test
        ;;
esac

log "✓ Load test complete"