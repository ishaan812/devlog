.PHONY: build install clean test run-ingest run-ask run-worklog run-onboard

BINARY_NAME=devlog
BUILD_DIR=./bin

build:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/devlog

install:
	go install ./cmd/devlog

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
	go fmt ./...

# Lint code
lint:
	golangci-lint run

# Verbose build
build-verbose:
	go build -v -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/devlog
