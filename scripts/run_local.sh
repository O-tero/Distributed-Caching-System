#!/usr/bin/env bash
# run_local.sh - Start local development environment
#
# Purpose: Orchestrate infrastructure startup and Encore service launch
#
# Usage:
#   ./scripts/run_local.sh                 # Start everything
#   ./scripts/run_local.sh --no-infra      # Skip infra (already running)
#   ./scripts/run_local.sh --foreground    # Run Encore in foreground
#   ./scripts/run_local.sh --stop          # Stop everything
#
# Features:
#   - Starts Docker Compose infrastructure
#   - Waits for service health checks
#   - Launches Encore dev server
#   - Prints service URLs
#   - Graceful shutdown handling

set -euo pipefail

# ============================================================================
# CONFIGURATION
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
INFRA_DIR="$PROJECT_ROOT/infra/local"

START_INFRA=1
FOREGROUND=0
STOP_MODE=0

# Service URLs
POSTGRES_PORT="${POSTGRES_PORT:-5432}"
REDIS_PORT="${REDIS_PORT:-6379}"
PROMETHEUS_PORT="${PROMETHEUS_PORT:-9090}"
ENCORE_PORT="${ENCORE_PORT:-9400}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

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

success() {
    echo -e "${GREEN}✓${NC} $*"
}

usage() {
    cat <<EOF
Usage: $0 [options]

Start local development environment for distributed caching system.

Options:
    --no-infra          Skip starting Docker infrastructure
    --foreground, -f    Run Encore in foreground (default: background)
    --stop              Stop all services and infrastructure
    --help, -h          Show this help

Examples:
    $0                          # Start everything
    $0 --no-infra               # Only start Encore (infra running)
    $0 --foreground             # Start with Encore in foreground
    $0 --stop                   # Stop everything

Service URLs (once started):
    Encore Dashboard:    http://localhost:$ENCORE_PORT
    Prometheus:          http://localhost:$PROMETHEUS_PORT
    PostgreSQL:          localhost:$POSTGRES_PORT
    Redis:               localhost:$REDIS_PORT

EOF
    exit 0
}

check_dependencies() {
    local missing=0
    
    log "Checking dependencies..."
    
    # Check Docker
    if ! command -v docker >/dev/null 2>&1; then
        error "Docker not found. Install from: https://docs.docker.com/engine/install/"
        missing=1
    else
        success "Docker found: $(docker --version)"
    fi
    
    # Check Docker Compose
    if ! docker compose version >/dev/null 2>&1; then
        error "Docker Compose V2 not found. Upgrade Docker to latest version."
        missing=1
    else
        success "Docker Compose found: $(docker compose version)"
    fi
    
    # Check Encore
    if ! command -v encore >/dev/null 2>&1; then
        error "Encore CLI not found. Install from: https://encore.dev/docs/install"
        error "Run: curl -L https://encore.dev/install.sh | bash"
        missing=1
    else
        success "Encore CLI found: $(encore version 2>/dev/null || echo 'installed')"
    fi
    
    if [[ $missing -eq 1 ]]; then
        error "Missing required dependencies. Please install them first."
        exit 1
    fi
}

start_infrastructure() {
    log "Starting Docker infrastructure..."
    
    cd "$INFRA_DIR"
    
    # Check if .env exists
    if [[ ! -f "$PROJECT_ROOT/.env" ]]; then
        warn ".env not found. Using default values from docker-compose.yml"
        warn "Consider creating .env from .env.example for customization"
    fi
    
    # Start services
    log "Running: docker compose up -d"
    if ! docker compose up -d; then
        error "Failed to start infrastructure"
        exit 1
    fi
    
    success "Infrastructure containers started"
    
    # Wait for services
    log "Waiting for services to be healthy..."
    
    # Wait for Postgres
    log "Checking PostgreSQL..."
    if ! "$SCRIPT_DIR/wait-for.sh" "localhost:$POSTGRES_PORT" --timeout 60 --quiet; then
        error "PostgreSQL failed to start"
        docker compose logs postgres
        exit 1
    fi
    success "PostgreSQL ready"
    
    # Wait for Redis
    log "Checking Redis..."
    if ! "$SCRIPT_DIR/wait-for.sh" "localhost:$REDIS_PORT" --timeout 60 --quiet; then
        error "Redis failed to start"
        docker compose logs redis
        exit 1
    fi
    success "Redis ready"
    
    # Wait for Prometheus
    log "Checking Prometheus..."
    if ! "$SCRIPT_DIR/wait-for.sh" "http://localhost:$PROMETHEUS_PORT/-/ready" --timeout 60 --quiet; then
        warn "Prometheus not ready (non-critical, continuing...)"
    else
        success "Prometheus ready"
    fi
    
    cd "$PROJECT_ROOT"
}

start_encore() {
    log "Starting Encore development server..."
    
    cd "$PROJECT_ROOT"
    
    if [[ $FOREGROUND -eq 1 ]]; then
        log "Running Encore in foreground (Ctrl+C to stop)"
        log "Command: encore run"
        echo ""
        
        # Setup trap for graceful shutdown
        trap 'log "Shutting down..."; exit 0' INT TERM
        
        encore run
    else
        log "Running Encore in background"
        log "Command: encore daemon start && encore run &"
        
        # Start Encore daemon if not running
        encore daemon start 2>/dev/null || true
        
        # Run in background
        nohup encore run > "$PROJECT_ROOT/encore.log" 2>&1 &
        ENCORE_PID=$!
        
        log "Encore PID: $ENCORE_PID"
        log "Logs: tail -f $PROJECT_ROOT/encore.log"
        
        # Wait a bit and check if it started
        sleep 3
        if ! kill -0 $ENCORE_PID 2>/dev/null; then
            error "Encore failed to start. Check logs: $PROJECT_ROOT/encore.log"
            exit 1
        fi
        
        success "Encore started in background"
    fi
}

print_urls() {
    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${GREEN}✓ Local Development Environment Ready${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    echo -e "${YELLOW}Service URLs:${NC}"
    echo -e "  Encore Dashboard:  ${BLUE}http://localhost:$ENCORE_PORT${NC}"
    echo -e "  Prometheus:        ${BLUE}http://localhost:$PROMETHEUS_PORT${NC}"
    echo -e "  PostgreSQL:        ${BLUE}localhost:$POSTGRES_PORT${NC}"
    echo -e "  Redis:             ${BLUE}localhost:$REDIS_PORT${NC}"
    echo ""
    echo -e "${YELLOW}Quick Commands:${NC}"
    echo -e "  Seed cache:        ${BLUE}./scripts/seed_data.sh${NC}"
    echo -e "  Load test:         ${BLUE}./scripts/load_test.sh${NC}"
    echo -e "  View logs:         ${BLUE}cd infra/local && docker compose logs -f${NC}"
    echo -e "  Stop services:     ${BLUE}$0 --stop${NC}"
    echo ""
    echo -e "${YELLOW}Example API Calls:${NC}"
    echo -e "  PUT cache entry:   ${BLUE}curl -X PUT http://localhost:$ENCORE_PORT/api/cache/test-key -d '{\"value\":\"data\"}'${NC}"
    echo -e "  GET cache entry:   ${BLUE}curl http://localhost:$ENCORE_PORT/api/cache/test-key${NC}"
    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
}

stop_services() {
    log "Stopping services..."
    
    # Stop Encore
    log "Stopping Encore..."
    pkill -f "encore run" || true
    encore daemon stop 2>/dev/null || true
    success "Encore stopped"
    
    # Stop Docker infrastructure
    log "Stopping Docker infrastructure..."
    cd "$INFRA_DIR"
    docker compose down
    success "Infrastructure stopped"
    
    log "All services stopped"
    log "To remove volumes: cd infra/local && docker compose down -v"
}

# ============================================================================
# MAIN
# ============================================================================

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --no-infra)
            START_INFRA=0
            shift
            ;;
        --foreground|-f)
            FOREGROUND=1
            shift
            ;;
        --stop)
            STOP_MODE=1
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

# Handle stop mode
if [[ $STOP_MODE -eq 1 ]]; then
    stop_services
    exit 0
fi

# Check dependencies
check_dependencies

# Start infrastructure if requested
if [[ $START_INFRA -eq 1 ]]; then
    start_infrastructure
else
    log "Skipping infrastructure startup (--no-infra)"
fi

# Start Encore
start_encore

# Print URLs (only if running in background)
if [[ $FOREGROUND -eq 0 ]]; then
    print_urls
    
    log "Development environment ready!"
    log "To stop: $0 --stop"
fi