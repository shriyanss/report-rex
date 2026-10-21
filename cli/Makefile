.PHONY: embed build build-server test lint clean

VERSION := $(shell cat ../version.txt 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/shriyanss/report-rex/cli/internal/version.Version=$(VERSION)

embed:
	bash scripts/prepare-embed.sh

build: embed
	go build -ldflags "$(LDFLAGS)" -o bin/rex .

build-server: embed
	go build -ldflags "$(LDFLAGS)" -o bin/rex-svr ./cmd/server

test: embed
	go build ./...
	go test ./... -cover

lint: embed
	gofmt -l .
	go vet ./...

clean:
	rm -rf bin internal/embedded/data
