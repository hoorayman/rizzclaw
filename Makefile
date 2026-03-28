.PHONY: all build clean build-linux build-linux-arm build-windows build-darwin build-all

APP_NAME := rizzclaw
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS := -ldflags "-s -w -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

GO := go
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)

all: build

build:
	$(GO) build $(LDFLAGS) -o $(APP_NAME) .

build-static:
	CGO_ENABLED=0 $(GO) build $(LDFLAGS) -o $(APP_NAME) .

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build $(LDFLAGS) -o bin/$(APP_NAME)-linux-amd64 .

build-linux-static:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build $(LDFLAGS) -tags netgo -installsuffix netgo -o bin/$(APP_NAME)-linux-amd64-static .

build-linux-arm:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 $(GO) build $(LDFLAGS) -o bin/$(APP_NAME)-linux-arm32v7 .

build-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build $(LDFLAGS) -o bin/$(APP_NAME)-linux-arm64 .

build-linux-arm-static:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 $(GO) build $(LDFLAGS) -tags netgo -installsuffix netgo -o bin/$(APP_NAME)-linux-arm32v7-static .

build-linux-arm64-static:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build $(LDFLAGS) -tags netgo -installsuffix netgo -o bin/$(APP_NAME)-linux-arm64-static .

build-windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build $(LDFLAGS) -o bin/$(APP_NAME)-windows-amd64.exe .

build-windows-arm64:
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 $(GO) build $(LDFLAGS) -o bin/$(APP_NAME)-windows-arm64.exe .

build-darwin:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GO) build $(LDFLAGS) -o bin/$(APP_NAME)-darwin-amd64 .

build-darwin-arm64:
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build $(LDFLAGS) -o bin/$(APP_NAME)-darwin-arm64 .

build-rpi:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=6 $(GO) build $(LDFLAGS) -o bin/$(APP_NAME)-rpi-arm32v6 .

build-rpi-static:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=6 $(GO) build $(LDFLAGS) -tags netgo -installsuffix netgo -o bin/$(APP_NAME)-rpi-arm32v6-static .

build-all: build-linux build-linux-arm build-linux-arm64 build-windows build-darwin build-darwin-arm64
	@echo "All builds completed in bin/"

build-all-static: build-linux-static build-linux-arm-static build-linux-arm64-static build-windows build-darwin build-darwin-arm64
	@echo "All static builds completed in bin/"

clean:
	rm -rf bin/
	rm -f $(APP_NAME) $(APP_NAME).exe

test:
	$(GO) test -v ./...

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

lint:
	golangci-lint run ./...

install:
	$(GO) install $(LDFLAGS) .

version:
	@echo "Version: $(VERSION)"
	@echo "Build Time: $(BUILD_TIME)"

help:
	@echo "RizzClaw Build System"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  build              Build for current platform"
	@echo "  build-static       Build static binary (no CGO) for current platform"
	@echo ""
	@echo "  Linux Builds:"
	@echo "    build-linux          Build for Linux AMD64"
	@echo "    build-linux-static   Build static binary for Linux AMD64"
	@echo "    build-linux-arm      Build for Linux ARM32 (Raspberry Pi 2/3/4)"
	@echo "    build-linux-arm-static  Build static binary for Linux ARM32"
	@echo "    build-linux-arm64    Build for Linux ARM64 (Raspberry Pi 4/5)"
	@echo "    build-linux-arm64-static Build static binary for Linux ARM64"
	@echo "    build-rpi            Build for Raspberry Pi Zero/1 (ARMv6)"
	@echo "    build-rpi-static     Build static binary for Raspberry Pi Zero/1"
	@echo ""
	@echo "  Windows Builds:"
	@echo "    build-windows        Build for Windows AMD64"
	@echo "    build-windows-arm64  Build for Windows ARM64"
	@echo ""
	@echo "  macOS Builds:"
	@echo "    build-darwin         Build for macOS AMD64"
	@echo "    build-darwin-arm64   Build for macOS ARM64 (Apple Silicon)"
	@echo ""
	@echo "  Build All:"
	@echo "    build-all            Build for all platforms"
	@echo "    build-all-static     Build static binaries for all platforms"
	@echo ""
	@echo "  Utilities:"
	@echo "    clean                Remove build artifacts"
	@echo "    test                 Run tests"
	@echo "    fmt                  Format code"
	@echo "    vet                  Run go vet"
	@echo "    lint                 Run golangci-lint"
	@echo "    install              Install binary to GOPATH/bin"
	@echo "    version              Show version info"
	@echo "    help                 Show this help message"
