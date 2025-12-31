#!/bin/bash
# =============================================================================
# Run Indexer with Maximum Resources
# =============================================================================
# Bu script, indexer çalışmadan önce diğer container'ları durdurur
# ve işlem bitince geri başlatır.
# =============================================================================

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Temp file to store stopped containers
STOPPED_CONTAINERS_FILE="/tmp/project-indexer-stopped-containers.txt"

# =============================================================================
# Functions
# =============================================================================

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

show_usage() {
    echo "Usage: $0 [OPTIONS] --project=<project-name>"
    echo ""
    echo "Options:"
    echo "  --project=NAME    Project to index (required)"
    echo "  --full            Force full reindex (delete existing data)"
    echo "  --sources=PATH    Override sources path (default: current directory)"
    echo "  --keep-others     Don't stop other containers"
    echo "  --no-restart      Don't restart stopped containers after indexing"
    echo "  --help            Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0 --project=my-project"
    echo "  $0 --project=my-project --full --sources=/path/to/code"
    echo "  $0 --project=project-indexer --sources=\$(pwd)"
}

get_other_containers() {
    # Get all running containers except project-indexer-* ones
    docker ps --format '{{.Names}}' | grep -v '^project-indexer-' || true
}

stop_other_containers() {
    log_info "Checking for other running containers..."
    
    local containers
    containers=$(get_other_containers)
    
    if [ -z "$containers" ]; then
        log_info "No other containers running"
        echo "" > "$STOPPED_CONTAINERS_FILE"
        return
    fi
    
    log_warn "Found other running containers:"
    echo "$containers" | while read -r name; do
        echo "  - $name"
    done
    
    log_info "Stopping other containers to free resources..."
    echo "$containers" > "$STOPPED_CONTAINERS_FILE"
    
    echo "$containers" | while read -r name; do
        if [ -n "$name" ]; then
            log_info "Stopping $name..."
            docker stop "$name" > /dev/null 2>&1 || true
        fi
    done
    
    log_success "Other containers stopped"
}

restart_stopped_containers() {
    if [ ! -f "$STOPPED_CONTAINERS_FILE" ]; then
        return
    fi
    
    local containers
    containers=$(cat "$STOPPED_CONTAINERS_FILE")
    
    if [ -z "$containers" ]; then
        rm -f "$STOPPED_CONTAINERS_FILE"
        return
    fi
    
    log_info "Restarting previously stopped containers..."
    
    echo "$containers" | while read -r name; do
        if [ -n "$name" ]; then
            log_info "Starting $name..."
            docker start "$name" > /dev/null 2>&1 || log_warn "Failed to start $name"
        fi
    done
    
    rm -f "$STOPPED_CONTAINERS_FILE"
    log_success "Containers restarted"
}

cleanup() {
    local exit_code=$?
    
    if [ "$NO_RESTART" != "true" ]; then
        restart_stopped_containers
    fi
    
    exit $exit_code
}

# =============================================================================
# Main
# =============================================================================

# Parse arguments
PROJECT=""
FULL_INDEX=""
SOURCES_PATH=""
KEEP_OTHERS="false"
NO_RESTART="false"

for arg in "$@"; do
    case $arg in
        --project=*)
            PROJECT="${arg#*=}"
            shift
            ;;
        --full)
            FULL_INDEX="--full"
            shift
            ;;
        --sources=*)
            SOURCES_PATH="${arg#*=}"
            shift
            ;;
        --keep-others)
            KEEP_OTHERS="true"
            shift
            ;;
        --no-restart)
            NO_RESTART="true"
            shift
            ;;
        --help|-h)
            show_usage
            exit 0
            ;;
        *)
            log_error "Unknown option: $arg"
            show_usage
            exit 1
            ;;
    esac
done

# Validate required arguments
if [ -z "$PROJECT" ]; then
    log_error "Project name is required"
    show_usage
    exit 1
fi

# Set default sources path
if [ -z "$SOURCES_PATH" ]; then
    SOURCES_PATH="$(pwd)"
fi

# Register cleanup handler
trap cleanup EXIT

# Change to project directory
cd "$PROJECT_ROOT"

echo ""
echo "=============================================="
echo "  Project Indexer - Resource Optimizer"
echo "=============================================="
echo ""
log_info "Project: $PROJECT"
log_info "Sources: $SOURCES_PATH"
log_info "Full Index: ${FULL_INDEX:-no}"
echo ""

# Stop other containers (unless --keep-others)
if [ "$KEEP_OTHERS" != "true" ]; then
    stop_other_containers
    echo ""
fi

# Ensure our services are running
log_info "Starting indexer dependencies..."
docker compose up -d qdrant ollama
echo ""

# Wait for services to be ready
log_info "Waiting for services to be ready..."
sleep 3

# Show resource usage
log_info "Current resource usage:"
docker stats --no-stream --format "table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}" | head -10
echo ""

# Run indexer
log_info "Starting indexer..."
echo ""

SOURCES_PATH="$SOURCES_PATH" docker compose run --rm indexer --project="$PROJECT" $FULL_INDEX

echo ""
log_success "Indexing complete!"
echo ""

# Cleanup will restart containers via trap
