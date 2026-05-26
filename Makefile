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

.PHONY: all build install test test-race vet tidy clean demo demo-workflow help

all: build

## build         Compile ./clu in the repo root.
build:
	$(GO) build -o $(BIN) $(PKG)

## install       Build and install to $(GOPATH)/bin/clu.
install:
	$(GO) install $(PKG)
	@echo "installed $(GOBIN)/$(BIN)"
	@case ":$$PATH:" in *":$(GOBIN):"*) ;; \
	    *) echo "WARNING: $(GOBIN) is not on your PATH. Add this to your shell rc:"; \
	       echo '    export PATH="$$HOME/go/bin:$$PATH"' ;; \
	esac

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
