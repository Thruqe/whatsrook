# Binary name configuration
ifeq ($(OS),Windows_NT)
    BINARY_NAME = whatsrook.exe
    CLEAN_CMD = if exist $(BINARY_NAME) del /q $(BINARY_NAME)
else
    BINARY_NAME = whatsrook
    CLEAN_CMD = rm -f $(BINARY_NAME)
endif

.PHONY: all build test fmt vet modernize proto clean help

# Default target runs everything
all: fmt vet test build

# Generate Protobuf code
proto:
	./scripts/generate-proto.sh

# Build the executable
build:
	go build -v -o ./tmp/$(BINARY_NAME) .

# Run unit tests
test:
	go test -v ./...

# Format the codebase
fmt:
	go fmt ./...

# Vet the codebase
vet:
	go vet ./...

# Run Go modernize analyzer to apply LSP/modern code fixes
modernize:
	go run golang.org/x/tools/go/analysis/passes/modernize/cmd/modernize@latest -fix ./...

# Clean build files
clean:
	go clean
	$(CLEAN_CMD)

# Display help menu
help:
	@echo "WhatsRook Makefile Targets:"
	@echo "  all       - Format, vet, run tests, and build the binary (default)"
	@echo "  build     - Compile the application binary"
	@echo "  test      - Run unit tests"
	@echo "  fmt       - Run go fmt on all packages"
	@echo "  vet       - Run go vet on all packages"
	@echo "  modernize - Automatically apply modern Go code fixes project-wide"
	@echo "  clean     - Clean up build files and executables"
	@echo "  help      - Show this help message"