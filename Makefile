.PHONY: build run test clean docker-build docker-up docker-down index

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod
BINARY_DIR=bin

# Build targets
build: build-indexer build-retrieval

build-indexer:
	$(GOBUILD) -o $(BINARY_DIR)/indexer ./cmd/indexer

build-retrieval:
	$(GOBUILD) -o $(BINARY_DIR)/retrieval-tool ./cmd/retrieval-tool

# Run locally
run-retrieval:
	$(GOBUILD) -o $(BINARY_DIR)/retrieval-tool ./cmd/retrieval-tool
	CONFIG_PATH=./configs/config.yaml ./$(BINARY_DIR)/retrieval-tool

# Test
test:
	$(GOTEST) -v ./...

test-coverage:
	$(GOTEST) -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out

# Dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

# Clean
clean:
	rm -rf $(BINARY_DIR)
	rm -f coverage.out

# Docker
docker-build:
	docker-compose build

docker-up:
	docker-compose up -d

docker-down:
	docker-compose down

docker-logs:
	docker-compose logs -f

# Indexing shortcuts
index:
	@if [ -z "$(PROJECT)" ]; then \
		echo "Usage: make index PROJECT=myproject [SOURCES=/path/to/code]"; \
		exit 1; \
	fi
	SOURCES_PATH=$(or $(SOURCES),./sources) docker compose run --rm indexer --project=$(PROJECT)

index-full:
	@if [ -z "$(PROJECT)" ]; then \
		echo "Usage: make index-full PROJECT=myproject [SOURCES=/path/to/code]"; \
		exit 1; \
	fi
	SOURCES_PATH=$(or $(SOURCES),./sources) docker compose run --rm indexer --project=$(PROJECT) --full

index-all:
	SOURCES_PATH=$(or $(SOURCES),./sources) docker compose run --rm indexer --all

# Optimized indexing (stops other containers for maximum resources)
index-optimized:
	@if [ -z "$(PROJECT)" ]; then \
		echo "Usage: make index-optimized PROJECT=myproject [SOURCES=/path/to/code]"; \
		exit 1; \
	fi
	./scripts/run-indexer.sh --project=$(PROJECT) --sources=$(or $(SOURCES),$(shell pwd))

index-optimized-full:
	@if [ -z "$(PROJECT)" ]; then \
		echo "Usage: make index-optimized-full PROJECT=myproject [SOURCES=/path/to/code]"; \
		exit 1; \
	fi
	./scripts/run-indexer.sh --project=$(PROJECT) --sources=$(or $(SOURCES),$(shell pwd)) --full

# Self-index this project (for testing)
index-self:
	./scripts/run-indexer.sh --project=project-indexer --sources=$(shell pwd)

index-self-full:
	./scripts/run-indexer.sh --project=project-indexer --sources=$(shell pwd) --full

# =============================================================================
# BEE PROJECTS
# =============================================================================
# Index all bee projects
index-bee:
	./scripts/index-bee-projects.sh

index-bee-full:
	./scripts/index-bee-projects.sh --full

# Index specific bee project
index-bee-hive2:
	./scripts/run-indexer.sh --project=bee-hive2 --sources=$$HOME/projects/bee

index-bee-queen:
	./scripts/run-indexer.sh --project=bee-queen --sources=$$HOME/projects/bee

index-bee-flora:
	./scripts/run-indexer.sh --project=bee-flora --sources=$$HOME/projects/bee

index-bee-app:
	./scripts/run-indexer.sh --project=bee-app-frontend --sources=$$HOME/projects/bee

# Setup
setup:
	cp -n configs/config.yaml.example configs/config.yaml || true
	mkdir -p data/index-cache
	mkdir -p sources
	@echo "Setup complete. Edit configs/config.yaml and add project configs."

# Pull embedding model
pull-model:
	docker-compose exec ollama ollama pull nomic-embed-text

# Hot reload config
reload:
	docker kill -s HUP $$(docker-compose ps -q retrieval-tool)

# Help
help:
	@echo "Available targets:"
	@echo "  build          - Build all binaries"
	@echo "  run-retrieval  - Run retrieval tool locally"
	@echo "  test           - Run tests"
	@echo "  docker-up      - Start all services"
	@echo "  docker-down    - Stop all services"
	@echo "  index          - Index a project (PROJECT=name required)"
	@echo "  index-full     - Full reindex a project"
	@echo "  index-all      - Index all configured projects"
	@echo "  setup          - Initial project setup"
	@echo "  pull-model     - Download embedding model"
	@echo "  reload         - Hot reload config"
