GO ?= go
ARCH_LINT ?= go-arch-lint
GOLANGCI_LINT ?= golangci-lint
ATAGO ?= atago
ACTIONLINT ?= actionlint

.PHONY: test lint workflows arch api api-update graphics e2e e2e-graphics cli-build build

# One module: ./... includes the gozen command and its internal implementations.
# Keep ONNX-dependent runtime tests separate; see docs/development/testing.md.
test:
	$(GO) test ./...

lint: arch api workflows
	$(GOLANGCI_LINT) run ./...
	$(GOLANGCI_LINT) fmt --diff

workflows:
	$(ACTIONLINT)

arch:
	$(ARCH_LINT) check

api:
	$(GO) test ./api -count=1

api-update:
	$(GO) run ./internal/dev/apigen

# Requires a display; Linux CI wraps this target with xvfb-run.
graphics:
	$(GO) test -tags=graphics ./runner -run TestGraphicsScreens -count=1

cli-build:
	mkdir -p bin
	$(GO) build -trimpath -buildvcs=false -ldflags=-compressdwarf=false -o bin/gozen ./cmd/gozen

e2e: cli-build
	mkdir -p build/atago
	cd build/atago && PATH="$(CURDIR)/bin:$$PATH" $(ATAGO) run --ci --parallel 4 --skip-tag graphics ../../e2e/atago

e2e-graphics: cli-build
	$(GO) build -trimpath -buildvcs=false -ldflags=-compressdwarf=false -o bin/gozen-game ./examples/play
	mkdir -p build/atago
	cd build/atago && PATH="$(CURDIR)/bin:$$PATH" GOZEN_GAME_BIN="$(CURDIR)/bin/gozen-game" $(ATAGO) run --ci --parallel 1 --tag graphics ../../e2e/atago

build:
	$(GO) build -trimpath -buildvcs=false ./...
