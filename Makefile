.PHONY: build install install-local clean test run-ingest run-ask run-worklog run-onboard

BINARY_NAME=devlog
BUILD_DIR=./bin
INSTALL_DIR=/usr/local/bin

build:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/devlog

# Install to $GOPATH/bin (usually ~/go/bin)
install:
	go install ./cmd/devlog

# Install to /usr/local/bin (requires sudo)
install-local: build
	sudo cp $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "Installed to $(INSTALL_DIR)/$(BINARY_NAME)"

# Uninstall from /usr/local/bin
uninstall:
	sudo rm -f $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "Removed $(INSTALL_DIR)/$(BINARY_NAME)"

clean:
	rm -rf $(BUILD_DIR)
	rm -rf ~/.devlog/devlog.db

clean-all:
	rm -rf $(BUILD_DIR)
	rm -rf ~/.devlog

test:
	go test -v ./...

# Development helpers
run-ingest:
	go run ./cmd/devlog ingest .

run-ask:
	go run ./cmd/devlog ask "What did I work on recently?"

run-worklog:
	go run ./cmd/devlog worklog --days 7 --no-llm

run-onboard:
	go run ./cmd/devlog onboard

# Build for multiple platforms
build-all:
	GOOS=darwin GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/devlog
	GOOS=darwin GOARCH=arm64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/devlog
	GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/devlog
	GOOS=windows GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/devlog

# Tidy dependencies
tidy:
	go mod tidy

# Format code
fmt:
	@echo "Running gofmt..."
	gofmt -s -w .
	@echo "Running goimports..."
	goimports -w -local github.com/ishaan812/devlog .

# Lint code
lint:
	golangci-lint run ./...

# Lint and fix
lint-fix:
	golangci-lint run --fix ./...

# Check formatting without modifying
fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "Files need formatting:" && gofmt -l . && exit 1)

# Run all checks
check: fmt-check lint test

# Pre-commit hook setup
pre-commit:
	@echo "Installing pre-commit hook..."
	@echo '#!/bin/sh\nmake fmt-check && make lint' > .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "Pre-commit hook installed"

# Verbose build
build-verbose:
	go build -v -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/devlog
