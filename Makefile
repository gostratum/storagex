# Makefile for StorageX development

.PHONY: help build test test-unit test-integration clean deps up down logs fmt lint

# Default target
help: ## Show this help
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# Development setup
deps: ## Install dependencies
	go mod download
	go mod tidy

build: ## Build the project
	go build ./...

# Testing
test: test-unit test-integration ## Run all tests

test-unit: ## Run unit tests
	go test -v -race -short ./...

test-integration: ## Run integration tests (requires MinIO)
	@echo "Starting MinIO for integration tests..."
	@docker compose -f tools/docker-compose.yml up -d minio
	@echo "Waiting for MinIO to be ready..."
	@sleep 10
	go test -v -race -tags=integration ./test/...
	@echo "Stopping MinIO..."
	@docker compose -f tools/docker-compose.yml down

test-coverage: ## Run tests with coverage
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
	goimports -w .

lint: ## Run linter
	golangci-lint run

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
	docker compose -f tools/docker-compose.yml down
	docker volume prune -f

# Release
release: clean deps test ## Prepare for release
	@echo "Running full test suite..."
	@$(MAKE) test
	@echo "Release preparation complete"

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

# Environment setup for different scenarios
env-minio: ## Set up environment for MinIO testing
	@echo "export STRATUM_STORAGE_ENDPOINT=http://localhost:9000"
	@echo "export STRATUM_STORAGE_ACCESS_KEY=minioadmin"
	@echo "export STRATUM_STORAGE_SECRET_KEY=minioadmin"
	@echo "export STRATUM_STORAGE_BUCKET=test-bucket"
	@echo "export STRATUM_STORAGE_USE_PATH_STYLE=true"
	@echo "export STRATUM_STORAGE_DISABLE_SSL=true"

env-localstack: ## Set up environment for Localstack testing
	@echo "export STRATUM_STORAGE_ENDPOINT=http://localhost:4566"
	@echo "export STRATUM_STORAGE_ACCESS_KEY=test"
	@echo "export STRATUM_STORAGE_SECRET_KEY=test"
	@echo "export STRATUM_STORAGE_BUCKET=test-bucket"
	@echo "export STRATUM_STORAGE_USE_PATH_STYLE=true"
	@echo "export STRATUM_STORAGE_DISABLE_SSL=true"
	@echo "export STRATUM_STORAGE_REGION=us-east-1"

env-aws: ## Set up environment for AWS S3
	@echo "export STRATUM_STORAGE_REGION=us-east-1"
	@echo "export STRATUM_STORAGE_BUCKET=your-s3-bucket"
	@echo "# Set STRATUM_STORAGE_ACCESS_KEY and STRATUM_STORAGE_SECRET_KEY with your AWS credentials"
	@echo "# Or use AWS IAM roles/profiles"

# Install development tools
install-tools: ## Install development tools
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Database operations for testing
reset-test-data: ## Reset test data in MinIO
	@echo "Resetting test data..."
	@docker compose -f tools/docker-compose.yml exec minio mc rm --recursive --force minio/test-bucket/
	@docker compose -f tools/docker-compose.yml exec minio mc rm --recursive --force minio/storagex-test/
	@echo "Test data reset complete"