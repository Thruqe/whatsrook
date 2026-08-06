# WhatsRook Makefile

.PHONY: start install format vet test update build patch clean help

# Default target
.DEFAULT_GOAL := help

# Colors
CYAN  := \033[36m
RESET := \033[0m

# Fallback match rule for positional target arguments (e.g. make start 2348060598064)
%:
	@:

help: ## Show this help message
	@echo "WhatsRook Makefile Commands:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(CYAN)%-15s$(RESET) %s\n", $$1, $$2}'

start: ## Run the WhatsRook CLI application (e.g. make start ARGS="-s <phone>" or make start -s <phone>)
	@sh entrypoint.sh $(filter-out start,$(MAKECMDGOALS)) $(ARGS)

install: ## Download and tidy Go module dependencies
	go mod download
	go mod tidy

format: ## Format all Go source files
	go fmt ./... && gofmt -w -s .

vet: ## Run go vet static code analysis
	go vet ./...

test: ## Run the test suite
	go test -v ./...

update: ## Upgrade all Go dependencies to their latest versions
	go get -u ./...
	go mod tidy

build: ## Build binary executable into bin/whatsrook
	mkdir -p bin
	go build -v -o bin/whatsrook ./cli

patch: ## Re-apply WhatsRook custom patches to whatsmeow/sqlstore
	bash patch/apply_patches.sh

clean: ## Remove build artifacts and temporary files
	rm -rf bin/ tmp/
