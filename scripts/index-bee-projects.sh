#!/bin/bash
# =============================================================================
# Index All Bee Projects
# =============================================================================
# Tüm bee projelerini sırayla indexler.
# Diğer container'ları durdurur, tüm projeler bittikten sonra geri başlatır.
# =============================================================================

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
STOPPED_CONTAINERS_FILE="/tmp/project-indexer-stopped-containers.txt"

# Bee projects source path
BEE_PATH="${BEE_PATH:-$HOME/projects/bee}"

# All bee projects
BEE_PROJECTS=(
    "bee-hive2"
    "bee-queen"
    "bee-app-frontend"
    "bee-flora"
)

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_header() { echo -e "${CYAN}$1${NC}"; }

show_usage() {
    echo "Usage: $0 [OPTIONS] [project1 project2 ...]"
    echo ""
    echo "Options:"
    echo "  --full            Force full reindex for all projects"
    echo "  --bee-path=PATH   Override bee projects path (default: ~/projects/bee)"
    echo "  --keep-others     Don't stop other containers"
    echo "  --list            List available projects"
    echo "  --help            Show this help"
    echo ""
    echo "Examples:"
    echo "  $0                      # Index all bee projects"
    echo "  $0 bee-hive2 bee-queen  # Index specific projects"
    echo "  $0 --full               # Full reindex all projects"
}

get_other_containers() {
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
    
    log_warn "Stopping other containers to free resources..."
    echo "$containers" > "$STOPPED_CONTAINERS_FILE"
    
    echo "$containers" | while read -r name; do
        if [ -n "$name" ]; then
            docker stop "$name" > /dev/null 2>&1 || true
        fi
    done
    
    log_success "Other containers stopped"
}

restart_stopped_containers() {
    if [ ! -f "$STOPPED_CONTAINERS_FILE" ]; then return; fi
    
    local containers
    containers=$(cat "$STOPPED_CONTAINERS_FILE")
    
    if [ -z "$containers" ]; then
        rm -f "$STOPPED_CONTAINERS_FILE"
        return
    fi
    
    log_info "Restarting previously stopped containers..."
    
    echo "$containers" | while read -r name; do
        if [ -n "$name" ]; then
            docker start "$name" > /dev/null 2>&1 || true
        fi
    done
    
    rm -f "$STOPPED_CONTAINERS_FILE"
    log_success "Containers restarted"
}

cleanup() {
    local exit_code=$?
    if [ "$KEEP_OTHERS" != "true" ]; then
        restart_stopped_containers
    fi
    exit $exit_code
}

# =============================================================================
# Main
# =============================================================================

FULL_INDEX=""
KEEP_OTHERS="false"
SELECTED_PROJECTS=()

for arg in "$@"; do
    case $arg in
        --full)
            FULL_INDEX="--full"
            ;;
        --bee-path=*)
            BEE_PATH="${arg#*=}"
            ;;
        --keep-others)
            KEEP_OTHERS="true"
            ;;
        --list)
            echo "Available projects:"
            for p in "${BEE_PROJECTS[@]}"; do
                echo "  - $p"
            done
            exit 0
            ;;
        --help|-h)
            show_usage
            exit 0
            ;;
        bee-*)
            SELECTED_PROJECTS+=("$arg")
            ;;
        *)
            log_error "Unknown option: $arg"
            show_usage
            exit 1
            ;;
    esac
done

# If no specific projects selected, use all
if [ ${#SELECTED_PROJECTS[@]} -eq 0 ]; then
    SELECTED_PROJECTS=("${BEE_PROJECTS[@]}")
fi

# Validate bee path
if [ ! -d "$BEE_PATH" ]; then
    log_error "Bee projects path not found: $BEE_PATH"
    exit 1
fi

trap cleanup EXIT

cd "$PROJECT_ROOT"

echo ""
log_header "=============================================="
log_header "  Bee Projects Indexer"
log_header "=============================================="
echo ""
log_info "Bee Path: $BEE_PATH"
log_info "Projects: ${#SELECTED_PROJECTS[@]}"
log_info "Full Index: ${FULL_INDEX:-no}"
echo ""

# Stop other containers
if [ "$KEEP_OTHERS" != "true" ]; then
    stop_other_containers
    echo ""
fi

# Start dependencies
log_info "Starting indexer dependencies..."
docker compose up -d qdrant ollama
sleep 3
echo ""

# Track results
declare -A RESULTS
TOTAL_FILES=0
TOTAL_CHUNKS=0
FAILED=0

# Index each project
for project in "${SELECTED_PROJECTS[@]}"; do
    echo ""
    log_header "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log_header "  Indexing: $project"
    log_header "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    
    START_TIME=$(date +%s)
    
    if SOURCES_PATH="$BEE_PATH" docker compose run --rm indexer --project="$project" $FULL_INDEX 2>&1; then
        END_TIME=$(date +%s)
        DURATION=$((END_TIME - START_TIME))
        RESULTS[$project]="✅ Success (${DURATION}s)"
        log_success "$project indexed in ${DURATION}s"
    else
        RESULTS[$project]="❌ Failed"
        ((FAILED++))
        log_error "$project indexing failed"
    fi
done

# Summary
echo ""
log_header "=============================================="
log_header "  Indexing Summary"
log_header "=============================================="
echo ""

for project in "${SELECTED_PROJECTS[@]}"; do
    echo "  $project: ${RESULTS[$project]}"
done

echo ""
if [ $FAILED -eq 0 ]; then
    log_success "All ${#SELECTED_PROJECTS[@]} projects indexed successfully!"
else
    log_warn "$FAILED of ${#SELECTED_PROJECTS[@]} projects failed"
fi
echo ""
