# LZ4 Go Build Orchestration
# Run `just --list` to see all available commands

# Default recipe - show available commands
default:
    @just --list

# Build commands

# Build the library
build:
    @echo "Building lz4 library..."
    go build ./...

# Test commands

# Run all tests
test:
    @echo "Running tests..."
    go test ./...

# Run tests with race detection
test-race:
    @echo "Running tests with race detection..."
    go test -race ./...

# Run tests with coverage
test-coverage:
    @echo "Running tests with coverage..."
    go test -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html
    @echo "Coverage report generated: coverage.html"

# Run benchmark tests
bench:
    @echo "Running benchmark tests..."
    go test -bench=. -benchmem ./...

# Run fuzz tests (short)
fuzz:
    @echo "Running fuzz tests..."
    go test -fuzz=. -fuzztime=30s ./fuzz/...

# Development commands

# Format all Go code
fmt:
    @echo "Formatting Go code..."
    treefmt --allow-missing-formatter

# Run go vet
vet:
    @echo "Running go vet..."
    go vet ./...

# Run linters (golangci-lint and treefmt)
lint:
    @echo "Running golangci-lint..."
    golangci-lint run --new ./...
    @echo "Running treefmt..."
    treefmt --fail-on-change

# Fix linting issues automatically
lint-fix:
    @echo "Fixing golangci-lint issues..."
    golangci-lint run --fix ./...
    @echo "Formatting with treefmt..."
    treefmt

# Tidy dependencies
tidy:
    @echo "Tidying dependencies..."
    go mod tidy

# Run all checks (fmt, vet, lint, tidy, test)
check: fmt vet lint tidy test

# Quick development check (fast feedback)
quick: fmt vet
    @echo "Quick check complete"

# Clean commands

# Clean build artifacts
clean:
    @echo "Cleaning build artifacts..."
    go clean ./...
    rm -f coverage.out coverage.html
    find . -name "*.test" -delete

# Continuous Integration commands

# CI build (strict mode)
ci-build:
    @echo "CI Build - strict mode"
    go build -race ./...

# CI test (with race detection)
ci-test:
    @echo "CI Test - with race detection"
    go test -race -timeout=10m ./...

# Full CI pipeline
ci: ci-build ci-test test-coverage
    @echo "CI pipeline completed"

# Git helpers

# Pre-commit hook
pre-commit: check
    @echo "Pre-commit checks passed!"

# Initialize git hooks
init-hooks:
    @echo "Setting up git hooks..."
    echo '#!/bin/sh\njust pre-commit' > .git/hooks/pre-commit
    chmod +x .git/hooks/pre-commit
    @echo "Git hooks installed"

# Fix all auto-fixable issues
fix: lint-fix fmt
