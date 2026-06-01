PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin

COMMANDS := localpager notifier-enqueue-github notifier-ingest-json notifier-watch notifier-worker

.PHONY: all build install test

all: build

build:
	@mkdir -p bin
	@for cmd in $(COMMANDS); do \
		go build -o bin/$$cmd ./cmd/$$cmd; \
	done

install: build
	@mkdir -p $(BINDIR)
	@for cmd in $(COMMANDS); do \
		install -m 0755 bin/$$cmd $(BINDIR)/$$cmd; \
	done

test:
	go test ./...
