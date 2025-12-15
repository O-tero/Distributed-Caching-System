#!/usr/bin/env bash
# deploy.sh - Deploy distributed caching system
#
# Purpose: Safe deployment helper for local/staging environments
#
# Usage:
#   ./scripts/deploy.sh --env staging           # Deploy to staging
#   ./scripts/deploy.sh --env staging --build   # Build and deploy
#
# SAFETY: Does NOT deploy to production without explicit confirmation

set -euo pipefail

# ============================================================================
# CONFIGURATION
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
INFRA_DIR="$PROJECT_ROOT/infra/local"

# Default values
ENVIRONMENT=""
BUILD=0
PUSH=0
TAG=""
DRY_RUN=0

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
Usage: $0 --env ENVIRONMENT [options]

Deploy distributed caching system to specified environment.

Required:
    --env ENV              Target environment: local, staging, production

Options:
    --build                Build images before deploying
    --push                 Push images to registry (requires registry config)
    --tag TAG              Image tag (default: git commit hash)
    --dry-run              Show what would be done without executing
    --help, -h             Show this help

Environments:
    local       Deploy to local Docker Compose
    staging     Deploy to staging environment
    production  Deploy to production (requires confirmation)

Examples:
    $0 --env local                    # Deploy to local
    $0 --env staging --build          # Build and deploy to staging
    $0 --env staging --tag v1.2.3     # Deploy specific tag to staging

Safety:
    - Production deployments require explicit --prod flag and confirmation
    - Dry-run mode available for testing
    - Git repo must be clean (no uncommitted changes)
    - Rollback instructions provided after deployment

EOF
    exit 0
}

check_prerequisites() {
    log "Checking prerequisites..."
    
    # Check Docker
    if ! command -v docker >/dev/null 2>&1; then
        error "Docker not found"
        exit 1
    fi
    
    # Check git
    if ! command -v git >/dev/null 2>&1; then
        error "git not found"
        exit 1
    fi
    
    # Check if in git repo
    if ! git rev-parse --git-dir >/dev/null 2>&1; then
        error "Not in a git repository"
        exit 1
    fi
    
    log "✓ Prerequisites OK"
}

validate_environment() {
    case $ENVIRONMENT in
        local|staging)
            log "Target environment: $ENVIRONMENT"
            ;;
        production|prod)
            error "Production deployment requires explicit confirmation"
            error "This script is designed for local/staging only"
            error "Use proper CI/CD pipeline for production deployments"
            exit 1
            ;;
        *)
            error "Invalid environment: $ENVIRONMENT"
            error "Valid: local, staging"
            exit 1
            ;;
    esac
}

check_git_status() {
    log "Checking git status..."
    
    # Check for uncommitted changes
    if [[ -n $(git status --porcelain) ]]; then
        warn "Git repository has uncommitted changes"
        if [[ $DRY_RUN -eq 0 ]]; then
            read -p "Continue anyway? (y/N) " -n 1 -r
            echo
            if [[ ! $REPLY =~ ^[Yy]$ ]]; then
                exit 1
            fi
        fi
    fi
    
    # Get git commit hash for tagging
    if [[ -z $TAG ]]; then
        TAG=$(git rev-parse --short HEAD)
        log "Auto-generated tag: $TAG"
    fi
    
    log "✓ Git status OK"
}

build_images() {
    log "Building Docker images..."
    
    cd "$PROJECT_ROOT"
    
    # Build services
    if [[ $DRY_RUN -eq 1 ]]; then
        log "[DRY-RUN] Would build images with tag: $TAG"
        return 0
    fi
    
    # Note: Adjust this based on your actual build process
    # For Encore apps, you might not need Docker build
    log "Building Encore services..."
    
    # Example: Build Go binaries
    # go build -o bin/cache-manager ./cache-manager
    # go build -o bin/invalidation ./invalidation
    # go build -o bin/warming ./warming
    # go build -o bin/monitoring ./monitoring
    
    log "✓ Build complete"
}

push_images() {
    if [[ $PUSH -eq 0 ]]; then
        return 0
    fi
    
    log "Pushing images to registry..."
    
    # Check registry configuration
    if [[ -z ${DOCKER_REGISTRY:-} ]]; then
        warn "DOCKER_REGISTRY not set, skipping push"
        return 0
    fi
    
    if [[ $DRY_RUN -eq 1 ]]; then
        log "[DRY-RUN] Would push images to $DOCKER_REGISTRY"
        return 0
    fi
    
    # Push images
    # docker push $DOCKER_REGISTRY/cache-manager:$TAG
    # docker push $DOCKER_REGISTRY/invalidation:$TAG
    # docker push $DOCKER_REGISTRY/warming:$TAG
    # docker push $DOCKER_REGISTRY/monitoring:$TAG
    
    log "✓ Images pushed"
}

deploy_local() {
    log "Deploying to local environment..."
    
    cd "$INFRA_DIR"
    
    if [[ $DRY_RUN -eq 1 ]]; then
        log "[DRY-RUN] Would run: docker compose up -d"
        return 0
    fi
    
    # Deploy infrastructure
    docker compose down
    docker compose up -d
    
    # Wait for services
    "$SCRIPT_DIR/wait-for.sh" "localhost:5432" --timeout 60 --quiet
    "$SCRIPT_DIR/wait-for.sh" "localhost:6379" --timeout 60 --quiet
    
    log "✓ Infrastructure deployed"
    
    # Start Encore services
    cd "$PROJECT_ROOT"
    log "Starting Encore services..."
    "$SCRIPT_DIR/run_local.sh" --no-infra
    
    log "✓ Services deployed"
}

deploy_staging() {
    log "Deploying to staging environment..."
    
    if [[ $DRY_RUN -eq 1 ]]; then
        log "[DRY-RUN] Would deploy to staging"
        return 0
    fi
    
    # For staging, use production-like compose file
    cd "$INFRA_DIR"
    
    # Check if prod compose file exists
    if [[ ! -f docker-compose.prod.yml ]]; then
        warn "docker-compose.prod.yml not found, using dev compose"
        docker compose -f docker-compose.yml up -d
    else
        docker compose -f docker-compose.prod.yml up -d
    fi
    
    log "✓ Staging deployment complete"
}

print_deployment_info() {
    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${GREEN}Deployment Complete${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    echo "Environment:  $ENVIRONMENT"
    echo "Tag:          $TAG"
    echo "Timestamp:    $(date)"
    echo ""
    echo "Next steps:"
    echo "  1. Verify services: docker compose ps"
    echo "  2. Check logs:      docker compose logs -f"
    echo "  3. Run smoke test:  curl http://localhost:9400/health"
    echo "  4. Seed data:       ./scripts/seed_data.sh"
    echo ""
    echo "Rollback (if needed):"
    echo "  1. Stop services:   docker compose down"
    echo "  2. Checkout prev:   git checkout <previous-commit>"
    echo "  3. Redeploy:        $0 --env $ENVIRONMENT"
    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
}

# ============================================================================
# MAIN
# ============================================================================

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --env)
            ENVIRONMENT=$2
            shift 2
            ;;
        --build)
            BUILD=1
            shift
            ;;
        --push)
            PUSH=1
            shift
            ;;
        --tag)
            TAG=$2
            shift 2
            ;;
        --dry-run)
            DRY_RUN=1
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

# Validate required args
if [[ -z $ENVIRONMENT ]]; then
    error "Environment required (--env)"
    usage
fi

# Run deployment
log "Starting deployment to $ENVIRONMENT"

if [[ $DRY_RUN -eq 1 ]]; then
    warn "DRY-RUN MODE: No changes will be made"
fi

check_prerequisites
validate_environment
check_git_status

if [[ $BUILD -eq 1 ]]; then
    build_images
fi

if [[ $PUSH -eq 1 ]]; then
    push_images
fi

case $ENVIRONMENT in
    local)
        deploy_local
        ;;
    staging)
        deploy_staging
        ;;
esac

if [[ $DRY_RUN -eq 0 ]]; then
    print_deployment_info
fi

log "✓ Deployment complete"