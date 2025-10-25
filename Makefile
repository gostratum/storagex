## Consolidated Makefile for storagex
.PHONY: help deps build test test-unit test-integration test-coverage fmt lint vet clean install-tools \
	up down up-localstack logs logs-minio run-example bench reset-test-data dev docs release version validate-version \
	update-deps bump-patch bump-minor bump-major release-dry-run release-patch release-minor release-major health env-minio env-localstack env-aws

VERSION := $(shell cat .version 2>/dev/null || echo "0.0.0")

# Default target: show help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# Development setup
deps: ## Download module dependencies
	go mod download
	go mod tidy

build: ## Build the project
	go build ./...

# Testing
test: test-unit test-integration ## Run unit and integration tests

test-unit: ## Run unit tests (short)
	go test -v -race -short ./...

test-integration: ## Run integration tests (requires MinIO)
	@echo "Starting MinIO for integration tests..."
	@docker compose -f tools/docker-compose.yml up -d minio
	@echo "Waiting for MinIO to be ready..."
	@sleep 10
	go test -v -race -tags=integration ./...
	@echo "Stopping MinIO..."
	@docker compose -f tools/docker-compose.yml down

test-coverage: ## Run tests with coverage and generate HTML report
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Docker services
up: ## Start MinIO and other services
	docker compose -f tools/docker-compose.yml up -d

down: ## Stop all services
	docker compose -f tools/docker-compose.yml down

up-localstack: ## Start Localstack instead of MinIO
	docker compose -f tools/docker-compose.yml --profile localstack up -d localstack

logs: ## Show service logs
	docker compose -f tools/docker-compose.yml logs -f

logs-minio: ## Show MinIO logs
	docker compose -f tools/docker-compose.yml logs -f minio

# Code quality
fmt: ## Format code
	go fmt ./...
	@command -v goimports >/dev/null 2>&1 && goimports -w . || echo "goimports not installed; run 'make install-tools'"

lint: ## Run linter (golangci-lint)
	@GOLANGCI_BIN=$(go env GOPATH)/bin/golangci-lint; \
	if [ -x "$$GOLANGCI_BIN" ]; then \
		"$$GOLANGCI_BIN" run ./...; \
	elif command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Run: make install-tools"; exit 1; \
	fi

vet: ## Run go vet
	go vet ./...

# Example and demo
run-example: ## Run the basic example (requires MinIO)
	@echo "Ensuring MinIO is running..."
	@docker compose -f tools/docker-compose.yml up -d minio
	@sleep 5
	cd examples/basic && go run main.go

# Benchmarks
bench: ## Run benchmarks
	go test -bench=. -benchmem ./...

# Clean up
clean: ## Clean up build artifacts and stop services
	go clean ./...
	docker compose -f tools/docker-compose.yml down || true
	docker volume prune -f || true
	rm -f coverage.out coverage.html

# # Release
# release: clean deps test ## Prepare for release
# 	@echo "Running full test suite..."
# 	@$(MAKE) test
# 	@echo "Release preparation complete"

# Documentation
docs: ## Generate documentation
	go doc -all .

# Quick development cycle
dev: up fmt test-unit ## Quick development cycle: start services, format, test

# Health check
health: ## Check if services are healthy
	@echo "Checking MinIO health..."
	@curl -f http://localhost:9000/minio/health/live || echo "MinIO is not healthy"
	@echo "Checking service status..."
	@docker compose -f tools/docker-compose.yml ps

# Environment helper targets
env-minio: ## Print environment exports for MinIO testing
	@echo "export STRATUM_STORAGE_ENDPOINT=http://localhost:9000"
	@echo "export STRATUM_STORAGE_ACCESS_KEY=minioadmin"
	@echo "export STRATUM_STORAGE_SECRET_KEY=minioadmin"
	@echo "export STRATUM_STORAGE_BUCKET=test-bucket"
	@echo "export STRATUM_STORAGE_USE_PATH_STYLE=true"
	@echo "export STRATUM_STORAGE_DISABLE_SSL=true"

env-localstack: ## Print environment exports for Localstack testing
	@echo "export STRATUM_STORAGE_ENDPOINT=http://localhost:4566"
	@echo "export STRATUM_STORAGE_ACCESS_KEY=test"
	@echo "export STRATUM_STORAGE_SECRET_KEY=test"
	@echo "export STRATUM_STORAGE_BUCKET=test-bucket"
	@echo "export STRATUM_STORAGE_USE_PATH_STYLE=true"
	@echo "export STRATUM_STORAGE_DISABLE_SSL=true"
	@echo "export STRATUM_STORAGE_REGION=us-east-1"

env-aws: ## Print environment exports for AWS S3
	@echo "export STRATUM_STORAGE_REGION=us-east-1"
	@echo "export STRATUM_STORAGE_BUCKET=your-s3-bucket"
	@echo "# Set STRATUM_STORAGE_ACCESS_KEY and STRATUM_STORAGE_SECRET_KEY with your AWS credentials or use IAM roles/profiles"

# Install development tools
install-tools: ## Install development tools used by the project
	@echo "Installing development tools..."
	@command -v goimports >/dev/null 2>&1 || \ 
		(go install golang.org/x/tools/cmd/goimports@latest)
	@command -v golangci-lint >/dev/null 2>&1 || \ 
		(go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	@echo "Tools installed (may need to add $(go env GOPATH)/bin to PATH)"

# Reset test data in MinIO
reset-test-data: ## Reset test data in MinIO (requires MinIO running via docker compose)
	@echo "Resetting test data..."
	@docker compose -f tools/docker-compose.yml exec minio mc rm --recursive --force minio/test-bucket/ || true
	@docker compose -f tools/docker-compose.yml exec minio mc rm --recursive --force minio/storagex-test/ || true
	@echo "Test data reset complete"

# Run tests (convenience)
test-all: ## Run all tests (alias for `test`)
	@$(MAKE) test

# Version management helpers
version: ## Print current version
	@echo "Current version: v$(VERSION)"

validate-version: ## Validate .version file if scripts exist
	@if [ -x ./scripts/validate-version.sh ]; then ./scripts/validate-version.sh; else echo "No validate-version script present"; fi

update-deps: ## Run update-deps script if present
	@if [ -x ./scripts/update-deps.sh ]; then ./scripts/update-deps.sh; else echo "No update-deps script present"; fi

bump-patch:
	@./scripts/bump-version.sh patch

bump-minor:
	@./scripts/bump-version.sh minor

bump-major:
	@./scripts/bump-version.sh major

# Release management (delegated to scripts if present)
release-dry-run:
	@DRY_RUN=true ./scripts/release.sh $(or $(TYPE),patch)

release-patch:
	@./scripts/release.sh patch

release-minor:
	@./scripts/release.sh minor

release-major:
	@./scripts/release.sh major

release: ## Run release script (default: patch)
	@./scripts/release.sh $(or $(TYPE),patch)