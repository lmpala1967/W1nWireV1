# W1nWire — developer Makefile (GNU make).
#
# Quick start:
#   make build          build server + client for the host platform
#   make test           run the unit tests
#   make cover          run the tests and open coverage.html
#   make lint           run golangci-lint (if installed)
#
# NOTE: recipe lines are indented with TAB characters (required by GNU make).

# ---------------------------------------------------------------------------
# Variables
# ---------------------------------------------------------------------------
MODULE      := github.com/w1nchi/w1nwire
BIN_DIR     := bin
SERVER_BIN  := $(BIN_DIR)/w1nwire-server
CLIENT_BIN  := $(BIN_DIR)/w1nwire-client
DESKTOP_BIN := $(BIN_DIR)/w1nwire-desktop.exe
DIST_DIR    := dist

# The icon and the resources of the Windows application.
ICON        := packaging/icon/w1nwire.ico
WINRES      ?= $(GO) run github.com/tc-hib/go-winres@v0.3.3

# Version stamped into the binaries via -X main.version=$(VERSION).
VERSION     ?= dev
LDFLAGS     := -s -w -X main.version=$(VERSION)

GO          ?= go
GOFLAGS     ?=
DOCKER      ?= docker
DOCKER_IMAGE ?= w1nwire:$(VERSION)
COMPOSE     ?= docker compose

# Server / client configuration examples.
SERVER_CONFIG ?= examples/server.json
CLIENT_CONFIG ?= examples/client.json

# Sub-commands used by the `keygen` and `conf` targets.
# Override them if the CLI flags change, e.g.
#   make keygen KEYGEN_CMD="go run ./cmd/w1nwire-server keygen -out examples/server.key"
KEYGEN_CMD  ?= $(GO) run ./cmd/w1nwire-server keygen
CONF_CMD    ?= $(GO) run ./cmd/w1nwire-client gen-config

.PHONY: all build build-linux build-arm64 desktop winres dist test test-race cover lint fmt fmt-check vet run-server run-client keygen conf docker-build docker-run clean tidy help

all: build

# ---------------------------------------------------------------------------
# Build
# ---------------------------------------------------------------------------
build: ## Build the server and the client for the host platform
	@mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -trimpath -ldflags "$(LDFLAGS)" -o $(SERVER_BIN) ./cmd/w1nwire-server
	$(GO) build $(GOFLAGS) -trimpath -ldflags "$(LDFLAGS)" -o $(CLIENT_BIN) ./cmd/w1nwire-client
	@echo "Built $(SERVER_BIN) and $(CLIENT_BIN) (version=$(VERSION))"

build-linux: ## Build linux/amd64 binaries
	@mkdir -p $(BIN_DIR)/linux-amd64
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -trimpath -ldflags "$(LDFLAGS)" \
		-o $(BIN_DIR)/linux-amd64/w1nwire-server ./cmd/w1nwire-server
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -trimpath -ldflags "$(LDFLAGS)" \
		-o $(BIN_DIR)/linux-amd64/w1nwire-client ./cmd/w1nwire-client

build-arm64: ## Build linux/arm64 binaries (Oracle Cloud Ampere instances)
	@mkdir -p $(BIN_DIR)/linux-arm64
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -trimpath -ldflags "$(LDFLAGS)" \
		-o $(BIN_DIR)/linux-arm64/w1nwire-server ./cmd/w1nwire-server
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -trimpath -ldflags "$(LDFLAGS)" \
		-o $(BIN_DIR)/linux-arm64/w1nwire-client ./cmd/w1nwire-client

winres: ## Generate the Windows resources of the desktop app (icon, manifest, version info)
	cd cmd/w1nwire-desktop && $(WINRES) simply --arch amd64 --out rsrc --manifest gui \
		--icon ../../$(ICON) \
		--product-version $(VERSION) --file-version $(VERSION) \
		--file-description "W1nWire VPN client" --product-name "W1nWire" \
		--original-filename w1nwire-desktop.exe --copyright "MIT License"

desktop: winres ## Build the Windows desktop application
	@mkdir -p $(BIN_DIR)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -trimpath -buildvcs=false \
		-ldflags "$(LDFLAGS) -H windowsgui" -o $(DESKTOP_BIN) ./cmd/w1nwire-desktop
	@echo "Built $(DESKTOP_BIN) (version=$(VERSION))"

dist: desktop ## Build every release artifact into dist/
	@mkdir -p $(DIST_DIR)
	cp $(DESKTOP_BIN) $(DIST_DIR)/
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -trimpath -buildvcs=false \
		-ldflags "$(LDFLAGS)" -o $(DIST_DIR)/w1nwire-server-linux-amd64 ./cmd/w1nwire-server
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -trimpath -buildvcs=false \
		-ldflags "$(LDFLAGS)" -o $(DIST_DIR)/w1nwire-server-linux-arm64 ./cmd/w1nwire-server
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -trimpath -buildvcs=false \
		-ldflags "$(LDFLAGS)" -o $(DIST_DIR)/w1nwire-client-linux-amd64 ./cmd/w1nwire-client
	cp deploy/install.sh $(DIST_DIR)/
	cd $(DIST_DIR) && sha256sum * > SHA256SUMS.txt
	@echo "Release artifacts are in $(DIST_DIR)/"

# ---------------------------------------------------------------------------
# Quality gates
# ---------------------------------------------------------------------------
test: ## Run the unit tests
	$(GO) test ./...

test-race: ## Run the unit tests with the race detector
	$(GO) test -race ./...

cover: ## Run the tests with coverage and produce coverage.html
	$(GO) test -coverprofile=coverage.out -covermode=atomic ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

lint: ## Run golangci-lint (falls back to go vet when the linter is missing)
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run --timeout=5m ./...; \
	else \
		echo "golangci-lint not found, running go vet instead"; \
		$(GO) vet ./...; \
	fi

fmt: ## Format the Go sources (gofmt)
	$(GO) fmt ./...

fmt-check: ## Fail if any Go file is not gofmt-formatted
	@unformatted="$$(gofmt -l . 2>&1)"; \
	if [ -n "$$unformatted" ]; then \
		echo "Not gofmt-formatted:"; echo "$$unformatted"; exit 1; \
	fi

vet: ## Run go vet
	$(GO) vet ./...

tidy: ## Tidy go.mod / go.sum
	$(GO) mod tidy

# ---------------------------------------------------------------------------
# Run
# ---------------------------------------------------------------------------
run-server: ## Run the VPN server with the example configuration
	$(GO) run ./cmd/w1nwire-server -config $(SERVER_CONFIG)

run-client: ## Run the VPN client with the example configuration
	$(GO) run ./cmd/w1nwire-client -config $(CLIENT_CONFIG)

keygen: ## Generate the server key pair
	$(KEYGEN_CMD)

conf: ## Generate the client configuration file
	$(CONF_CMD) -out $(CLIENT_CONFIG)

# ---------------------------------------------------------------------------
# Docker
# ---------------------------------------------------------------------------
docker-build: ## Build the Docker image
	$(DOCKER) build -t $(DOCKER_IMAGE) .

docker-run: ## Run the monitoring stack (Prometheus + Grafana)
	$(COMPOSE) -f monitoring/docker-compose.yml up -d

# ---------------------------------------------------------------------------
# Housekeeping
# ---------------------------------------------------------------------------
clean: ## Remove build artifacts and coverage output
	rm -rf $(BIN_DIR) coverage.out coverage.html
	$(GO) clean -cache -testcache 2>/dev/null || true

help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'
