# dist/<name>-<os>-<arch>

NAME := goapk
CMD := .

CGO_ENABLED ?= 0
GOFLAGS := -trimpath
LDFLAGS := -s -w

GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
BIN := dist/$(NAME)-$(GOOS)-$(GOARCH)

.PHONY: bin darwin-arm64 linux-arm64 linux-amd64 clean

bin:
	mkdir -p dist
	rm -rf $(BIN)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
	go build $(GOFLAGS) -ldflags '$(LDFLAGS)' \
	-o $(BIN) $(CMD)

darwin-arm64:
	$(MAKE) bin GOOS=darwin GOARCH=arm64

linux-arm64:
	$(MAKE) bin GOOS=linux GOARCH=arm64 \
	CC="zig cc -target aarch64-linux-gnu" \
	CXX="zig c++ -target aarch64-linux-gnu"

linux-amd64:
	$(MAKE) bin GOOS=linux GOARCH=amd64 \
	CC="zig cc -target x86_64-linux-gnu" \
	CXX="zig c++ -target x86_64-linux-gnu"

clean:
	rm -rf dist
