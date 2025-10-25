.PHONY: build run test clean docker-build docker-up docker-down install bench coverage

# Build settings
BINARY_NAME=proxy-checker
BUILD_DIR=./build
CMD_DIR=./cmd
GO_FILES=$(shell find . -name '*.go' -type f)

# Build the application
build:
	@echo "Building..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=1 go build -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)/main.go
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Run the application
run: build
	@echo "Starting application..."
	@$(BUILD_DIR)/$(BINARY_NAME)

# Run tests
test:
	@echo "Running tests..."
	go test ./... -v -race -cover

# Run tests with coverage
coverage:
	@echo "Running tests with coverage..."
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Run benchmarks
bench:
	@echo "Running benchmarks..."
	go test ./... -bench=. -benchmem

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html
	@echo "Clean complete"

# Docker build
docker-build:
	@echo "Building Docker image..."
	docker build -t proxy-checker:latest .

# Docker start
docker-up:
	@echo "Starting with Docker Compose..."
	docker-compose up -d
	@echo "Service started. Check status with: docker-compose ps"

# Docker stop
docker-down:
	@echo "Stopping Docker Compose..."
	docker-compose down

# Docker logs
docker-logs:
	docker-compose logs -f proxy-checker

# Install dependencies
deps:
	@echo "Installing dependencies..."
	go mod download
	go mod verify

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...
	gofmt -s -w .

# Lint code
lint:
	@echo "Linting..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not installed" && exit 1)
	golangci-lint run ./...

# Install to /opt/proxy-checker (requires sudo)
install: build
	@echo "Installing to /opt/proxy-checker..."
	sudo mkdir -p /opt/proxy-checker
	sudo cp $(BUILD_DIR)/$(BINARY_NAME) /opt/proxy-checker/
	sudo cp config.example.json /opt/proxy-checker/config.json
	sudo chown -R root:root /opt/proxy-checker
	sudo chmod +x /opt/proxy-checker/$(BINARY_NAME)
	@echo "Installation complete"

# System tuning (requires sudo)
tune:
	@echo "Applying system tuning..."
	@echo "Setting ulimit..."
	ulimit -n 65535 || true
	@echo "Applying sysctl settings..."
	sudo sysctl -w net.ipv4.ip_local_port_range="10000 65535" || true
	sudo sysctl -w net.ipv4.tcp_max_syn_backlog=8192 || true
	sudo sysctl -w net.ipv4.tcp_tw_reuse=1 || true
	sudo sysctl -w net.core.somaxconn=8192 || true
	@echo "Tuning complete. For permanent changes, see OPS_CHECKLIST.md"

# Quick setup
setup: deps build
	@echo "Setting up development environment..."
	@cp config.example.json config.json
	@cp env.example .env
	@echo "Setup complete. Edit config.json and .env, then run: make run"

# Help
help:
	@echo "Available targets:"
	@echo "  build        - Build the application"
	@echo "  run          - Build and run the application"
	@echo "  test         - Run tests"
	@echo "  coverage     - Run tests with coverage report"
	@echo "  bench        - Run benchmarks"
	@echo "  clean        - Remove build artifacts"
	@echo "  docker-build - Build Docker image"
	@echo "  docker-up    - Start with Docker Compose"
	@echo "  docker-down  - Stop Docker Compose"
	@echo "  docker-logs  - View Docker logs"
	@echo "  deps         - Install Go dependencies"
	@echo "  fmt          - Format code"
	@echo "  lint         - Lint code"
	@echo "  install      - Install to /opt/proxy-checker"
	@echo "  tune         - Apply system tuning"
	@echo "  setup        - Quick development setup"
	@echo "  help         - Show this help message"

