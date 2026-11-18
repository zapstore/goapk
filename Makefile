# dist/<name>-<os>-<arch>

NAME := goapk
CMD := .

CGO_ENABLED ?= 0
GOFLAGS := -trimpath
LDFLAGS := -s -w

GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
BIN := dist/$(NAME)-$(GOOS)-$(GOARCH)
LOCALBIN := $(HOME)/.local/bin

HOST_OS := $(shell go env GOOS)
HOST_ARCH := $(shell go env GOARCH)

.DEFAULT_GOAL := help
.PHONY: help build darwin-arm64 linux-arm64 linux-amd64 clean

help: ## List targets
	@awk 'BEGIN { FS = ":.*## " } /^[a-zA-Z0-9_.-]+:.*## / { printf "  %-20s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

build: ## Build dist/$(NAME)-$(GOOS)-$(GOARCH); link when that matches this machine
	mkdir -p dist
	rm -rf $(BIN)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
	go build $(GOFLAGS) -ldflags '$(LDFLAGS)' \
	-o $(BIN) $(CMD)
ifeq ($(GOOS)-$(GOARCH),$(HOST_OS)-$(HOST_ARCH))
	ln -sfn $(BIN) $(NAME)
	@if [ -d "$(LOCALBIN)" ]; then \
		ln -sfn "$(CURDIR)/$(BIN)" "$(LOCALBIN)/$(NAME)" || exit 1; \
		echo "linked $(LOCALBIN)/$(NAME)"; \
	fi
endif

darwin-arm64: ## macOS arm64 → dist/ only
	$(MAKE) build GOOS=darwin GOARCH=arm64

linux-arm64: ## Linux arm64 → dist/ only
	$(MAKE) build GOOS=linux GOARCH=arm64 \
	CC="zig cc -target aarch64-linux-gnu" \
	CXX="zig c++ -target aarch64-linux-gnu"

linux-amd64: ## Linux amd64 → dist/ only
	$(MAKE) build GOOS=linux GOARCH=amd64 \
	CC="zig cc -target x86_64-linux-gnu" \
	CXX="zig c++ -target x86_64-linux-gnu"

clean: ## Remove dist/ and symlinks
	rm -rf dist
	rm -f $(NAME)
	@l="$(LOCALBIN)/$(NAME)"; \
	if [ -L "$$l" ]; then \
		case "$$(readlink "$$l")" in "$(CURDIR)/dist/"*) rm -f "$$l";; esac; \
	fi
