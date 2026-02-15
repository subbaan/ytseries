.PHONY: build install clean test

VERSION := 2.5.4
BINDIR := $(HOME)/bin

build:
	go build -o bin/ytseries ./cmd/ytseries

install: build
	cp bin/ytseries $(BINDIR)/ytseries
	chmod +x $(BINDIR)/ytseries

clean:
	rm -rf bin/

test:
	go test -v ./...

fmt:
	go fmt ./...

vet:
	go vet ./...
