BINARY := nexus
BUILD_DIR := bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build test clean install dist universal docker-test

build:
	cp README.md cmd/nexus/embed/README.md
	CGO_ENABLED=1 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/nexus

dist:
	cp README.md cmd/nexus/embed/README.md
	CGO_ENABLED=1 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/nexus
	@echo "Distributable binary: $(BUILD_DIR)/$(BINARY) ($$(du -h $(BUILD_DIR)/$(BINARY) | cut -f1))"

universal:
	cp README.md cmd/nexus/embed/README.md
	@echo "Building arm64 (Apple Silicon)..."
	CGO_ENABLED=1 GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-arm64 ./cmd/nexus
	@echo "Building amd64 (Intel)..."
	CGO_ENABLED=1 GOARCH=amd64 CC="clang -arch x86_64" CXX="clang++ -arch x86_64" go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-amd64 ./cmd/nexus
	@echo "Creating universal binary..."
	lipo -create -output $(BUILD_DIR)/$(BINARY) $(BUILD_DIR)/$(BINARY)-arm64 $(BUILD_DIR)/$(BINARY)-amd64
	rm $(BUILD_DIR)/$(BINARY)-arm64 $(BUILD_DIR)/$(BINARY)-amd64
	@echo "Universal binary: $(BUILD_DIR)/$(BINARY) ($$(du -h $(BUILD_DIR)/$(BINARY) | cut -f1)) — runs on any Mac"

docker-test:
	docker build -f Dockerfile.test -t nexus-sandbox .
	docker run --rm nexus-sandbox

test:
	go test ./... -v -count=1

clean:
	rm -rf $(BUILD_DIR)

install:
	CGO_ENABLED=1 go install $(LDFLAGS) ./cmd/nexus
