# clu — common targets.
#
# Most-used flow:
#   make install      # build + put `clu` on PATH (via $(go env GOPATH)/bin)
#   make test         # run all tests
#   make demo         # exercise the tool end-to-end against a temp dir

GO       ?= go
BIN      := clu
PKG      := ./cmd/clu
GOBIN    := $(shell $(GO) env GOPATH)/bin

.PHONY: all build install install-bin install-web test test-race vet tidy clean demo demo-workflow help

all: build

## build         Compile ./clu in the repo root.
build:
	$(GO) build -o $(BIN) $(PKG)

## install       Build + install the clu binary AND the web UI bundle so
##               `clu web` works from any directory. Requires pnpm.
install: install-bin install-web

## install-bin   Install just the Go binary to $(GOPATH)/bin/clu.
install-bin:
	$(GO) install $(PKG)
	@echo "installed $(GOBIN)/$(BIN)"
	@case ":$$PATH:" in *":$(GOBIN):"*) ;; \
	    *) echo "WARNING: $(GOBIN) is not on your PATH. Add this to your shell rc:"; \
	       echo '    export PATH="$$HOME/go/bin:$$PATH"' ;; \
	esac

## install-web   Build the web UI (pnpm install + pnpm build) and copy
##               .output/ to ~/.local/share/clu/web. Skipped silently if
##               pnpm is missing — the binary still works without the UI.
install-web:
	@if command -v pnpm >/dev/null 2>&1; then \
	    $(GOBIN)/$(BIN) web --install; \
	else \
	    echo "skipping web install: pnpm not on PATH"; \
	    echo "  install pnpm and run: clu web --install"; \
	fi

## test          Run all tests (no cache).
test:
	$(GO) test -count=1 ./...

## test-race     Tests with -race; slower but catches data races.
test-race:
	$(GO) test -race -count=1 ./...

## vet           Static analysis.
vet:
	$(GO) vet ./...

## tidy          Tidy go.mod / go.sum.
tidy:
	$(GO) mod tidy

## clean         Remove the local build artefact.
clean:
	rm -f $(BIN)

## demo          End-to-end smoke (issues, deps, labels, defer, export/import).
demo: build
	./demo.sh

## demo-workflow End-to-end workflow templates smoke.
demo-workflow: build
	./demo-workflow.sh

## help          Print this list.
help:
	@awk '/^## / { sub(/^## /, ""); printf "  %s\n", $$0 }' $(MAKEFILE_LIST)
