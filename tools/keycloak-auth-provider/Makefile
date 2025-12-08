.PHONY: build clean test lint tidy

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X main.Version=$(VERSION)

build:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o bin/gptscript-go-tool .

clean:
	rm -rf bin/

test:
	go test -v -race -cover ./...

lint:
	golangci-lint run

tidy:
	go mod tidy
