#!/usr/bin/env bash
# backup_db.sh - Backup PostgreSQL database
#
# Purpose: Create timestamped database backups with rotation
#
# Usage:
#   ./scripts/backup_db.sh                  # Backup with auto timestamp
#   ./scripts/backup_db.sh --name custom    # Custom backup name
#   ./scripts/backup_db.sh --restore FILE   # Restore from backup
#
# Features:
#   - Timestamped backups
#   - Automatic rotation (keep last N backups)
#   - Compression (gzip)
#   - Restore capability
#   - Docker-aware (uses docker exec if pg_dump not available)

set -euo pipefail

# ============================================================================
# CONFIGURATION
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BACKUP_DIR="$PROJECT_ROOT/infra/local/backups"

# PostgreSQL settings
POSTGRES_HOST="${POSTGRES_HOST:-localhost}"
POSTGRES_PORT="${POSTGRES_PORT:-5432}"
POSTGRES_USER="${POSTGRES_USER:-cache_user}"
POSTGRES_DB="${POSTGRES_DB:-distributed_cache}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-changeme_dev_only}"

# Backup settings
BACKUP_NAME=""
RESTORE_FILE=""
RETENTION=7  # Keep last 7 backups
COMPRESS=1

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

Backup and restore PostgreSQL database.

Options:
    --name NAME            Custom backup name (default: timestamp)
    --restore FILE         Restore from backup file
    --no-compress          Don't compress backup
    --retention N          Keep last N backups (default: 7)
    --help, -h             Show this help

Examples:
    $0                              # Create timestamped backup
    $0 --name before_migration      # Named backup
    $0 --restore backup.sql.gz      # Restore from backup

Backup Location:
    $BACKUP_DIR/

Environment Variables:
    POSTGRES_HOST          PostgreSQL host (default: localhost)
    POSTGRES_PORT          PostgreSQL port (default: 5432)
    POSTGRES_USER          PostgreSQL user (default: cache_user)
    POSTGRES_PASSWORD      PostgreSQL password
    POSTGRES_DB            Database name (default: distributed_cache)

EOF
    exit 0
}

setup_backup_dir() {
    if [[ ! -d $BACKUP_DIR ]]; then
        log "Creating backup directory: $BACKUP_DIR"
        mkdir -p "$BACKUP_DIR"
    fi
}

check_postgres() {
    log "Checking PostgreSQL connection..."
    
    # Try psql if available
    if command -v psql >/dev/null 2>&1; then
        if PGPASSWORD="$POSTGRES_PASSWORD" psql \
            -h "$POSTGRES_HOST" \
            -p "$POSTGRES_PORT" \
            -U "$POSTGRES_USER" \
            -d "$POSTGRES_DB" \
            -c "SELECT 1" >/dev/null 2>&1; then
            log "✓ PostgreSQL connection OK (psql)"
            return 0
        fi
    fi
    
    # Try via Docker
    if docker ps --format '{{.Names}}' | grep -q postgres; then
        if docker exec cache_postgres psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "SELECT 1" >/dev/null 2>&1; then
            log "✓ PostgreSQL connection OK (docker)"
            return 0
        fi
    fi
    
    error "Cannot connect to PostgreSQL"
    error "Ensure database is running: cd infra/local && docker compose up -d postgres"
    exit 1
}

create_backup() {
    setup_backup_dir
    check_postgres
    
    # Generate backup filename
    local timestamp=$(date +%Y%m%d_%H%M%S)
    if [[ -n $BACKUP_NAME ]]; then
        local filename="backup_${BACKUP_NAME}_${timestamp}.sql"
    else
        local filename="backup_${timestamp}.sql"
    fi
    
    local backup_path="$BACKUP_DIR/$filename"
    
    log "Creating backup: $filename"
    log "Database: $POSTGRES_DB"
    log "Target: $backup_path"
    
    # Perform backup
    if command -v pg_dump >/dev/null 2>&1; then
        # Use pg_dump directly
        log "Using pg_dump..."
        PGPASSWORD="$POSTGRES_PASSWORD" pg_dump \
            -h "$POSTGRES_HOST" \
            -p "$POSTGRES_PORT" \
            -U "$POSTGRES_USER" \
            -d "$POSTGRES_DB" \
            --clean \
            --if-exists \
            --verbose \
            > "$backup_path" 2>&1
    else
        # Use Docker exec
        log "Using docker exec pg_dump..."
        docker exec cache_postgres pg_dump \
            -U "$POSTGRES_USER" \
            -d "$POSTGRES_DB" \
            --clean \
            --if-exists \
            > "$backup_path"
    fi
    
    if [[ ! -f $backup_path ]]; then
        error "Backup failed: file not created"
        exit 1
    fi
    
    # Compress if enabled
    if [[ $COMPRESS -eq 1 ]]; then
        log "Compressing backup..."
        gzip -f "$backup_path"
        backup_path="${backup_path}.gz"
    fi
    
    # Get backup size
    local size=$(du -h "$backup_path" | cut -f1)
    
    log "✓ Backup created successfully"
    log "File: $backup_path"
    log "Size: $size"
    
    # Rotate old backups
    rotate_backups
}

rotate_backups() {
    log "Rotating old backups (retention: $RETENTION)..."
    
    # Count backups
    local backup_count=$(find "$BACKUP_DIR" -name "backup_*.sql*" -type f | wc -l)
    
    if [[ $backup_count -le $RETENTION ]]; then
        log "✓ Backup count ($backup_count) within retention limit ($RETENTION)"
        return 0
    fi
    
    # Delete oldest backups
    local to_delete=$((backup_count - RETENTION))
    log "Deleting $to_delete old backup(s)..."
    
    find "$BACKUP_DIR" -name "backup_*.sql*" -type f -printf '%T+ %p\n' \
        | sort \
        | head -n $to_delete \
        | cut -d' ' -f2- \
        | while read file; do
            log "Removing: $(basename "$file")"
            rm -f "$file"
        done
    
    log "✓ Rotation complete"
}

restore_backup() {
    local backup_file="$1"
    
    # Resolve path
    if [[ ! -f $backup_file ]]; then
        # Try in backup directory
        backup_file="$BACKUP_DIR/$backup_file"
        if [[ ! -f $backup_file ]]; then
            error "Backup file not found: $1"
            exit 1
        fi
    fi
    
    log "Restoring from backup: $backup_file"
    
    # Check if compressed
    local restore_cmd
    if [[ $backup_file =~ \.gz$ ]]; then
        log "Backup is compressed, decompressing..."
        restore_cmd="gunzip -c"
    else
        restore_cmd="cat"
    fi
    
    # Confirm restore
    warn "This will OVERWRITE the current database: $POSTGRES_DB"
    read -p "Continue? (yes/NO) " -r
    if [[ ! $REPLY =~ ^yes$ ]]; then
        log "Restore cancelled"
        exit 0
    fi
    
    # Perform restore
    check_postgres
    
    if command -v psql >/dev/null 2>&1; then
        log "Restoring via psql..."
        $restore_cmd "$backup_file" | PGPASSWORD="$POSTGRES_PASSWORD" psql \
            -h "$POSTGRES_HOST" \
            -p "$POSTGRES_PORT" \
            -U "$POSTGRES_USER" \
            -d "$POSTGRES_DB"
    else
        log "Restoring via docker exec..."
        $restore_cmd "$backup_file" | docker exec -i cache_postgres psql \
            -U "$POSTGRES_USER" \
            -d "$POSTGRES_DB"
    fi
    
    log "✓ Restore complete"
}

list_backups() {
    setup_backup_dir
    
    echo ""
    echo -e "${BLUE}Available Backups:${NC}"
    echo ""
    
    if [[ ! -d $BACKUP_DIR ]] || [[ -z $(ls -A "$BACKUP_DIR" 2>/dev/null) ]]; then
        echo "No backups found in $BACKUP_DIR"
        return 0
    fi
    
    find "$BACKUP_DIR" -name "backup_*.sql*" -type f -printf '%T+ %s %p\n' \
        | sort -r \
        | while read timestamp size file; do
            local filename=$(basename "$file")
            local size_human=$(numfmt --to=iec-i --suffix=B $size 2>/dev/null || echo "$size bytes")
            local date=$(echo $timestamp | cut -d+ -f1)
            printf "  %-40s  %10s  %s\n" "$filename" "$size_human" "$date"
        done
    
    echo ""
}

# ============================================================================
# MAIN
# ============================================================================

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --name)
            BACKUP_NAME=$2
            shift 2
            ;;
        --restore)
            RESTORE_FILE=$2
            shift 2
            ;;
        --no-compress)
            COMPRESS=0
            shift
            ;;
        --retention)
            RETENTION=$2
            shift 2
            ;;
        --list)
            list_backups
            exit 0
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

# Perform action
if [[ -n $RESTORE_FILE ]]; then
    restore_backup "$RESTORE_FILE"
else
    create_backup
fi

log "✓ Done"