# Project variables
BINARY_NAME=eip7702cleaner
# Get version from git tag if available, otherwise use default
VERSION=$(shell git describe --tags 2>/dev/null || echo "0.1.0")
BUILD_DIR=build
MAIN_FILE=cmd/eip7702cleaner/main.go
GITHUB_REPO=github.com/ethanzhrepo/eip7702cleaner

# Build info
LDFLAGS=-ldflags "-X '${GITHUB_REPO}/pkg/cmd.Version=${VERSION}'"

# Default target - local build for current platform
.PHONY: default
default: clean build

# Clean build directory
.PHONY: clean
clean:
	@echo "Cleaning build directory..."
	@rm -rf $(BUILD_DIR)
	@mkdir -p $(BUILD_DIR)

# Build for the current platform
.PHONY: build
build:
	@echo "Building for current platform (version: $(VERSION))..."
	@go build -o $(BUILD_DIR)/$(BINARY_NAME) $(LDFLAGS) $(MAIN_FILE)
	@echo "Binary created at $(BUILD_DIR)/$(BINARY_NAME)"

# Run tests
.PHONY: test
test:
	@echo "Running tests..."
	@go test ./...

# Build for all platforms (macOS, Linux, Windows)
.PHONY: all
all: clean darwin linux windows

# Build for macOS (both amd64 and arm64)
.PHONY: darwin
darwin:
	@echo "Building for macOS (amd64)..."
	@GOOS=darwin GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)_darwin_amd64 $(LDFLAGS) $(MAIN_FILE)
	@echo "Building for macOS (arm64)..."
	@GOOS=darwin GOARCH=arm64 go build -o $(BUILD_DIR)/$(BINARY_NAME)_darwin_arm64 $(LDFLAGS) $(MAIN_FILE)

# Build for Linux (amd64)
.PHONY: linux
linux:
	@echo "Building for Linux (amd64)..."
	@GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)_linux_amd64 $(LDFLAGS) $(MAIN_FILE)
	@echo "Building for Linux (arm64)..."
	@GOOS=linux GOARCH=arm64 go build -o $(BUILD_DIR)/$(BINARY_NAME)_linux_arm64 $(LDFLAGS) $(MAIN_FILE)

# Build for Windows (amd64)
.PHONY: windows
windows:
	@echo "Building for Windows (amd64)..."
	@GOOS=windows GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)_windows_amd64.exe $(LDFLAGS) $(MAIN_FILE)

# Release - build all platforms and create compressed archives
.PHONY: release
release: all
	@echo "Creating release archives (version: $(VERSION))..."
	@cd $(BUILD_DIR) && tar -czf $(BINARY_NAME)_$(VERSION)_darwin_amd64.tar.gz $(BINARY_NAME)_darwin_amd64
	@cd $(BUILD_DIR) && tar -czf $(BINARY_NAME)_$(VERSION)_darwin_arm64.tar.gz $(BINARY_NAME)_darwin_arm64
	@cd $(BUILD_DIR) && tar -czf $(BINARY_NAME)_$(VERSION)_linux_amd64.tar.gz $(BINARY_NAME)_linux_amd64
	@cd $(BUILD_DIR) && tar -czf $(BINARY_NAME)_$(VERSION)_linux_arm64.tar.gz $(BINARY_NAME)_linux_arm64
	@cd $(BUILD_DIR) && zip $(BINARY_NAME)_$(VERSION)_windows_amd64.zip $(BINARY_NAME)_windows_amd64.exe
	@echo "Release archives created in $(BUILD_DIR) directory"

# Install locally
.PHONY: install
install: build
	@echo "Installing binary to GOPATH/bin..."
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/
	@echo "Installation complete"

# Create a new git tag for the current version
.PHONY: tag
tag:
	@echo "Creating git tag for version $(VERSION)..."
	@git tag -a $(VERSION) -m "Release version $(VERSION)"
	@echo "Tag created. Use 'git push --tags' to push to remote repository."

# Show current version
.PHONY: version
version:
	@echo "Current version: $(VERSION)"

# Help
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build    - Build for current platform"
	@echo "  clean    - Clean build directory"
	@echo "  test     - Run tests"
	@echo "  darwin   - Build for macOS (amd64 and arm64)"
	@echo "  linux    - Build for Linux (amd64 and arm64)"
	@echo "  windows  - Build for Windows (amd64)"
	@echo "  all      - Build for all platforms"
	@echo "  release  - Create release archives for all platforms"
	@echo "  install  - Install binary to GOPATH/bin"
	@echo "  tag      - Create a new git tag for the current version"
	@echo "  version  - Show current version"
	@echo "  help     - Show this help message" 