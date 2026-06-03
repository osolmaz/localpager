PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin

COMMANDS := localpager localpager-enqueue-github localpager-ingest-json localpager-watch localpager-worker

.PHONY: all build install test

all: build

build:
	@mkdir -p bin
	@for cmd in $(COMMANDS); do \
		go build -o bin/$$cmd ./cmd/$$cmd; \
	done
	cd reposhell && go build -o ../bin/reposhell ./cmd/reposhell

install: build
	@mkdir -p $(BINDIR)
	@for cmd in $(COMMANDS); do \
		install -m 0755 bin/$$cmd $(BINDIR)/$$cmd; \
	done
	install -m 0755 bin/reposhell $(BINDIR)/reposhell

test:
	go test $$(./scripts/go-packages.sh)
	cd reposhell && go test ./...
