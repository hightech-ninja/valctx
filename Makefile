VERSION := v0.0.2
COMMIT_HASH := $(shell git rev-parse --short HEAD 2>/dev/null || echo "main")
BUILD_DATE := $(shell date '+%Y-%m-%d')

.PHONY: build clean install test

build:
	@go build -a -ldflags \
	"-X 'main.Version=${VERSION}' -X 'main.CommitHash=${COMMIT_HASH}' -X 'main.BuildDate=${BUILD_DATE}'" \
	 ./cmd/valctx;

clean:
	@rm valctx;

install:
	@echo "VERSION=${VERSION}";
	@echo "BUILD_DATE=${BUILD_DATE}";
	@echo "COMMIT_HASH=${COMMIT_HASH}";
	@go install -a -ldflags \
	"-X 'main.Version=${VERSION}' -X 'main.CommitHash=${COMMIT_HASH}' -X 'main.BuildDate=${BUILD_DATE}' -s -w" \
	./cmd/valctx;

test:
	go test -v -race ./...;