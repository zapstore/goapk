BINARY_NAME := goapk
CMD_PATH    := .
DIST        := dist

GOFLAGS := -trimpath
LDFLAGS := -s -w

# Android SDK tools — only needed for `make dex` (maintainer only).
# d8 can be either the Android SDK binary or r8.jar run via Java.
# Detected automatically; override with D8= or R8_JAR= + JAVA=.
ANDROID_SDK ?= $(HOME)/Library/Android/sdk
BUILD_TOOLS ?= $(shell ls -1d $(ANDROID_SDK)/build-tools/* 2>/dev/null | sort -V | tail -1)
SDK_D8      := $(BUILD_TOOLS)/d8

JAVA        ?= $(shell find /opt/homebrew/Cellar/openjdk -name java -type f 2>/dev/null | head -1)
R8_JAR      ?= $(shell ls /tmp/goapk-compile/r8.jar 2>/dev/null || ls $(HOME)/.goapk/r8.jar 2>/dev/null)
ANDROID_JAR ?= $(shell ls /tmp/goapk-compile/android.jar 2>/dev/null || ls $(HOME)/.goapk/android.jar 2>/dev/null || ls $(ANDROID_SDK)/platforms/android-35/android.jar 2>/dev/null)

# Select d8 implementation: prefer SDK binary, fall back to r8.jar
ifeq ($(wildcard $(SDK_D8)),$(SDK_D8))
  D8_CMD = $(SDK_D8)
  DEX_COMPILE = $(D8_CMD) --release --min-api 24 --output /tmp/goapk-dex/ /tmp/goapk-javac/com/zapstore/goapk/runtime/*.class
else
  D8_CMD = $(JAVA) -cp $(R8_JAR) com.android.tools.r8.D8
  DEX_COMPILE = $(D8_CMD) --release --min-api 24 --lib $(ANDROID_JAR) --output /tmp/goapk-dex/ /tmp/goapk-javac/com/zapstore/goapk/runtime/*.class
endif

JAVA_SRCS   := $(wildcard java/*.java)
DEX_OUT     := internal/embed/classes.dex

.PHONY: all build build-darwin-arm64 build-linux-amd64 build-linux-arm64 \
        clean test install fmt vet dex genstub

build:
	CGO_ENABLED=0 go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BINARY_NAME) $(CMD_PATH)

all: build-darwin-arm64 build-linux-amd64 build-linux-arm64

build-darwin-arm64:
	@mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 \
		go build $(GOFLAGS) -ldflags '$(LDFLAGS)' \
		-o $(DIST)/$(BINARY_NAME)-darwin-arm64 $(CMD_PATH)

build-linux-amd64:
	@mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
		go build $(GOFLAGS) -ldflags '$(LDFLAGS)' \
		-o $(DIST)/$(BINARY_NAME)-linux-amd64 $(CMD_PATH)

build-linux-arm64:
	@mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
		go build $(GOFLAGS) -ldflags '$(LDFLAGS)' \
		-o $(DIST)/$(BINARY_NAME)-linux-arm64 $(CMD_PATH)

# Recompile the WebView activity DEX from Java source.
# Works with Android SDK d8 OR with r8.jar + android.jar (no full SDK needed).
# Prereqs without Android SDK:
#   mkdir -p ~/.goapk
#   curl -L ".../android-all-15-robolectric-12650502.jar" -o ~/.goapk/android.jar
#   curl -L "https://dl.google.com/dl/android/maven2/com/android/tools/r8/9.1.31/r8-9.1.31.jar" -o ~/.goapk/r8.jar
dex:
	@test -n "$(ANDROID_JAR)" || \
		{ echo "Error: android.jar not found. Set ANDROID_SDK or place it at ~/.goapk/android.jar"; exit 1; }
	@test -n "$(JAVA)" || \
		{ echo "Error: java not found (needed for r8 fallback). Install openjdk."; exit 1; }
	@echo "Compiling $(JAVA_SRCS) → $(DEX_OUT)"
	@mkdir -p $(dir $(DEX_OUT)) /tmp/goapk-javac /tmp/goapk-dex
	$(dir $(JAVA))javac --release 8 \
		-classpath "$(ANDROID_JAR)" \
		-d /tmp/goapk-javac \
		$(JAVA_SRCS)
	$(DEX_COMPILE)
	cp /tmp/goapk-dex/classes.dex $(DEX_OUT)
	@echo "Written: $(DEX_OUT)"
	@rm -rf /tmp/goapk-javac /tmp/goapk-dex

# Regenerate the minimal stub DEX (no classes). Safe to run at any time.
genstub:
	go run ./tools/genstub -o $(DEX_OUT)
	@echo "Written stub: $(DEX_OUT)"

clean:
	rm -f $(BINARY_NAME)
	rm -rf $(DIST)
	go clean

test:
	go test -v ./...

install:
	go install $(CMD_PATH)

fmt:
	go fmt ./...

vet:
	go vet ./...
