GO ?= go
ARCH_LINT ?= go-arch-lint
# gofmt's output changes between Go releases (Go 1.27 aligns map literals with
# wide characters differently from 1.26), and golangci-lint formats with the Go
# it was built with. So the linter is built and run with one pinned Go here and
# in the CI lint job (.github/workflows/ci.yml, which runs make lint), whatever
# Go is installed. go.mod's go line stays the minimum; tests keep running on the
# minimum and on stable. Bump LINT_GO together with the lint job's go-version.
LINT_GO ?= go1.27.2
GOLANGCI_LINT_VERSION ?= v2.14.0
GOLANGCI_LINT ?= GOTOOLCHAIN=$(LINT_GO) $(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
ATAGO ?= atago
ACTIONLINT ?= actionlint

.PHONY: go-zen go-zen-graphics test coverage lint workflows arch api api-update graphics e2e e2e-graphics cli-build build website website-serve website-check bench bench-compare docs docs-images docs-images-check docs-steps

# One module: ./... includes the gozen command and its internal implementations.
# Keep ONNX-dependent runtime tests separate; see docs/development/testing.md.
test:
	$(GO) test ./...

# coverage.out is read by octocov (.octocov.yml). -coverpkg counts tests in one
# package toward the packages they exercise.
coverage:
	$(GO) test -covermode=atomic -coverpkg=./... -coverprofile=coverage.out ./...

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

# examples/go-zen is a nested module (it replaces gozen with this checkout), so
# ./... does not reach it. go-zen needs no display; go-zen-graphics takes its
# screens and needs one (CI runs it in the graphics job under xvfb-run).
go-zen:
	cd examples/go-zen && $(GO) vet ./...
	$(GO) run ./cmd/gozen check examples/go-zen/game
	cd examples/go-zen && $(GO) test -count=1 ./...

go-zen-graphics:
	cd examples/go-zen && $(GO) test -tags=graphics -count=1 ./...

# Requires a display; Linux CI wraps this target with xvfb-run.
graphics:
	$(GO) test -tags=graphics ./play -run 'TestGraphicsScreens|TestGraphicsStage|TestGraphicsMap|TestGraphicsWild' -count=1
	$(GO) test -tags=graphics ./puppet -run TestGraphics -count=1

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

# Measure gozen with the himorime suite in bench/ (requires himorime on PATH).
bench:
	himorime run bench

# Compare main (or BASE) with the working tree on the himorime suite.
bench-compare:
	himorime compare --against $${BASE:-main} bench

# The documentation website (GitHub Pages). Requires hugo; see website/hugo.toml.
website:
	cd website && hugo --gc --minify --cleanDestinationDir

website-serve:
	cd website && hugo server

# Broken internal links and pictures of the built site (after make website).
website-check:
	GOZEN_SITE_DIR="$(CURDIR)/website/public" $(GO) test ./internal/dev/docs -run '^TestSite(Links|Schema)$$' -count=1

# README and site pictures and steps (docs/development/docs.md). The pictures
# come from the samples listed in docs/images/manifest.yaml and need a display:
# on Linux they run inside xvfb-run, so no window opens on your screen.
XVFB ?= $(shell command -v xvfb-run >/dev/null && echo 'xvfb-run -a -s "-screen 0 1920x1080x24"')
DOCS_TEST = $(GO) test ./internal/dev/docs -count=1 -timeout 40m

docs-images:
	GOZEN_DOCS_IMAGES=update $(XVFB) $(DOCS_TEST) -run '^TestDocImages$$' -v

docs-images-check:
	GOZEN_DOCS_IMAGES=check $(XVFB) $(DOCS_TEST) -run '^TestDocImages$$'

# Runs the shell steps of README.md and website/content against this checkout.
docs-steps:
	GOZEN_DOCS_STEPS=1 $(XVFB) $(DOCS_TEST) -run '^TestDocSteps$$'

docs: docs-images-check docs-steps website website-check
