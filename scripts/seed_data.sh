#!/usr/bin/env bash
# seed_data.sh - Populate cache and database with sample data
#
# Purpose: Create realistic test data for development and testing
#
# Usage:
#   ./scripts/seed_data.sh                    # Seed 100 entries
#   ./scripts/seed_data.sh --count 500        # Seed 500 entries
#   ./scripts/seed_data.sh --clean            # Clear existing data first
#
# Features:
#   - Generates realistic cache entries
#   - Populates PostgreSQL with audit records
#   - Creates varied data patterns (users, products, sessions)
#   - Idempotent (safe to re-run)

set -euo pipefail

# ============================================================================
# CONFIGURATION
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Default values
COUNT=100
CLEAN=0
CACHE_URL="${CACHE_MANAGER_URL:-http://localhost:9400}"
POSTGRES_HOST="${POSTGRES_HOST:-localhost}"
POSTGRES_PORT="${POSTGRES_PORT:-5432}"
POSTGRES_USER="${POSTGRES_USER:-cache_user}"
POSTGRES_DB="${POSTGRES_DB:-distributed_cache}"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
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

Populate cache and database with sample data for testing.

Options:
    --count N              Number of cache entries to create (default: 100)
    --clean                Clear existing data before seeding
    --cache-url URL        Cache manager URL (default: $CACHE_URL)
    --postgres-url URL     PostgreSQL connection URL
    --help, -h             Show this help

Examples:
    $0                          # Seed 100 entries
    $0 --count 500              # Seed 500 entries
    $0 --clean --count 200      # Clear and seed 200 entries

Environment Variables:
    CACHE_MANAGER_URL          Cache manager base URL
    POSTGRES_HOST              PostgreSQL host (default: localhost)
    POSTGRES_PORT              PostgreSQL port (default: 5432)
    POSTGRES_USER              PostgreSQL user (default: cache_user)
    POSTGRES_PASSWORD          PostgreSQL password
    POSTGRES_DB                PostgreSQL database (default: distributed_cache)

EOF
    exit 0
}

check_dependencies() {
    log "Checking dependencies..."
    
    # Check curl
    if ! command -v curl >/dev/null 2>&1; then
        error "curl not found. Install: sudo apt-get install curl"
        exit 1
    fi
    
    # Check jq (optional but helpful)
    if ! command -v jq >/dev/null 2>&1; then
        warn "jq not found (optional). Install for better output: sudo apt-get install jq"
    fi
}

wait_for_services() {
    log "Waiting for services..."
    
    # Wait for cache-manager
    log "Checking cache-manager at $CACHE_URL..."
    if ! "$SCRIPT_DIR/wait-for.sh" "$CACHE_URL/health" --timeout 60 --quiet 2>/dev/null; then
        warn "Cache manager health endpoint not available, trying base URL..."
        if ! "$SCRIPT_DIR/wait-for.sh" "$(echo $CACHE_URL | sed 's|http://||' | sed 's|/.*||')" --timeout 30 --quiet; then
            error "Cache manager not available at $CACHE_URL"
            exit 1
        fi
    fi
    log "✓ Cache manager ready"
    
    # Wait for PostgreSQL
    log "Checking PostgreSQL..."
    if ! "$SCRIPT_DIR/wait-for.sh" "$POSTGRES_HOST:$POSTGRES_PORT" --timeout 60 --quiet; then
        error "PostgreSQL not available at $POSTGRES_HOST:$POSTGRES_PORT"
        exit 1
    fi
    log "✓ PostgreSQL ready"
}

clean_data() {
    log "Cleaning existing data..."
    
    # Clear cache (if cache-manager has flush endpoint)
    log "Clearing cache entries..."
    curl -s -X DELETE "$CACHE_URL/api/cache" || true
    
    # Clear audit logs
    log "Clearing audit logs..."
    PGPASSWORD="${POSTGRES_PASSWORD:-changeme_dev_only}" psql \
        -h "$POSTGRES_HOST" \
        -p "$POSTGRES_PORT" \
        -U "$POSTGRES_USER" \
        -d "$POSTGRES_DB" \
        -c "TRUNCATE TABLE cache_system.invalidation_audit, cache_system.metrics_events CASCADE;" \
        2>/dev/null || true
    
    log "✓ Data cleaned"
}

generate_user_data() {
    local user_id=$1
    cat <<EOF
{
  "id": $user_id,
  "username": "user$user_id",
  "email": "user${user_id}@example.com",
  "firstName": "First${user_id}",
  "lastName": "Last${user_id}",
  "createdAt": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "preferences": {
    "theme": "dark",
    "notifications": true
  }
}
EOF
}

generate_product_data() {
    local product_id=$1
    local price=$((RANDOM % 10000 + 1000))
    cat <<EOF
{
  "id": $product_id,
  "name": "Product ${product_id}",
  "description": "High-quality product for your needs",
  "price": $(echo "scale=2; $price/100" | bc),
  "currency": "USD",
  "stock": $((RANDOM % 1000)),
  "category": "Category $((product_id % 10))",
  "tags": ["tag1", "tag2", "popular"]
}
EOF
}

generate_session_data() {
    local session_id=$1
    cat <<EOF
{
  "sessionId": "sess_${session_id}_$(date +%s)",
  "userId": $((session_id % 100 + 1)),
  "createdAt": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "expiresAt": "$(date -u -d '+1 hour' +%Y-%m-%dT%H:%M:%SZ)",
  "ipAddress": "192.168.$((RANDOM % 255)).$((RANDOM % 255))",
  "userAgent": "Mozilla/5.0 (X11; Linux x86_64)"
}
EOF
}

seed_cache_entries() {
    local count=$1
    log "Seeding $count cache entries..."
    
    local success=0
    local failed=0
    
    # Create progress bar
    local progress=0
    local bar_width=50
    
    for ((i=1; i<=count; i++)); do
        # Determine entry type (40% users, 30% products, 30% sessions)
        local rand=$((RANDOM % 100))
        local key
        local data
        
        if [[ $rand -lt 40 ]]; then
            # User entry
            key="users:$i"
            data=$(generate_user_data $i)
        elif [[ $rand -lt 70 ]]; then
            # Product entry
            key="products:$i"
            data=$(generate_product_data $i)
        else
            # Session entry
            key="sessions:$i"
            data=$(generate_session_data $i)
        fi
        
        # PUT to cache
        if curl -s -X PUT "$CACHE_URL/api/cache/$key" \
            -H "Content-Type: application/json" \
            -d "$data" \
            -w "%{http_code}" \
            -o /dev/null | grep -q "^2"; then
            ((success++))
        else
            ((failed++))
        fi
        
        # Update progress bar
        if [[ $((i % 10)) -eq 0 ]] || [[ $i -eq $count ]]; then
            progress=$((i * 100 / count))
            local filled=$((progress * bar_width / 100))
            local empty=$((bar_width - filled))
            
            printf "\r["
            printf "%${filled}s" | tr ' ' '='
            printf "%${empty}s" | tr ' ' ' '
            printf "] %3d%% (%d/%d)" "$progress" "$i" "$count"
        fi
    done
    
    echo ""
    log "✓ Seeded $success entries successfully, $failed failed"
}

seed_audit_logs() {
    log "Seeding audit logs..."
    
    # Create sample invalidation audit entries
    PGPASSWORD="${POSTGRES_PASSWORD:-changeme_dev_only}" psql \
        -h "$POSTGRES_HOST" \
        -p "$POSTGRES_PORT" \
        -U "$POSTGRES_USER" \
        -d "$POSTGRES_DB" \
        <<EOF
INSERT INTO cache_system.invalidation_audit (request_id, service, pattern, keys, triggered_by, triggered_at, metadata)
VALUES
    ('req-001', 'cache-manager', 'users:*', '["users:1", "users:2"]', 'api-gateway', NOW() - INTERVAL '1 hour', '{"reason": "user_update"}'),
    ('req-002', 'invalidation', 'products:*', NULL, 'admin-panel', NOW() - INTERVAL '2 hours', '{"reason": "price_change"}'),
    ('req-003', 'cache-manager', NULL, '["sessions:123", "sessions:456"]', 'auth-service', NOW() - INTERVAL '30 minutes', '{"reason": "logout"}'),
    ('req-004', 'warming', 'users:*', NULL, 'cron-job', NOW() - INTERVAL '10 minutes', '{"reason": "scheduled_refresh"}'),
    ('req-005', 'cache-manager', 'products:top*', NULL, 'api-gateway', NOW() - INTERVAL '5 minutes', '{"reason": "cache_refresh"}');
EOF
    
    log "✓ Audit logs seeded"
}

seed_warming_schedules() {
    log "Seeding warming schedules..."
    
    # Create sample warming schedules
    PGPASSWORD="${POSTGRES_PASSWORD:-changeme_dev_only}" psql \
        -h "$POSTGRES_HOST" \
        -p "$POSTGRES_PORT" \
        -U "$POSTGRES_USER" \
        -d "$POSTGRES_DB" \
        <<EOF
INSERT INTO cache_system.warming_schedules (name, description, cron_expression, key_pattern, priority, batch_size)
VALUES
    ('hourly_top_users', 'Warm top 100 user profiles every hour', '0 * * * *', 'users:top:*', 10, 100),
    ('daily_products', 'Warm all products daily at 2 AM', '0 2 * * *', 'products:*', 5, 500),
    ('frequent_sessions', 'Warm active sessions every 15 minutes', '*/15 * * * *', 'sessions:active:*', 20, 50)
ON CONFLICT (name) DO NOTHING;
EOF
    
    log "✓ Warming schedules seeded"
}

print_summary() {
    log "Seed complete! Summary:"
    echo ""
    echo "Cache entries:        $COUNT"
    echo "Cache URL:            $CACHE_URL"
    echo "PostgreSQL:           $POSTGRES_HOST:$POSTGRES_PORT"
    echo ""
    echo "Next steps:"
    echo "  1. View cache stats:  curl $CACHE_URL/api/stats"
    echo "  2. Run load test:     ./scripts/load_test.sh"
    echo "  3. Query audit logs:  psql -h $POSTGRES_HOST -U $POSTGRES_USER -d $POSTGRES_DB"
    echo ""
}

# ============================================================================
# MAIN
# ============================================================================

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --count)
            COUNT=$2
            shift 2
            ;;
        --clean)
            CLEAN=1
            shift
            ;;
        --cache-url)
            CACHE_URL=$2
            shift 2
            ;;
        --postgres-url)
            # Parse postgres://user:pass@host:port/db
            POSTGRES_URL=$2
            # TODO: Parse URL components
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

# Run seeding
log "Starting data seeding (count: $COUNT, clean: $CLEAN)"

check_dependencies
wait_for_services

if [[ $CLEAN -eq 1 ]]; then
    clean_data
fi

seed_cache_entries "$COUNT"
seed_audit_logs
seed_warming_schedules

print_summary

log "✓ Seeding complete!"