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
ZIZMOR ?= zizmor

.PHONY: website-player web-runtime e2e-web desktop-runtime website-go-zen website-novel website-campaign website-play-check go-zen go-zen-graphics novel novel-graphics campaign campaign-graphics test coverage lint tidy workflows arch boundary graphics e2e e2e-graphics cli-build build website website-serve website-check bench bench-compare docs docs-images docs-images-check docs-steps

# One module: ./... includes the gozen command and its internal implementations.
# Keep ONNX-dependent runtime tests separate; see docs/development/testing.md.
test:
	$(GO) test ./...

# coverage.out is read by octocov (.octocov.yml). -coverpkg counts tests in one
# package toward the packages they exercise. Include the real capture process:
# the Ebitengine loop cannot run inside the unit-test process.
coverage:
	mkdir -p build/atago
	@set -eu; \
	work=$$(mktemp -d "$(CURDIR)/build/coverage.XXXXXX"); \
	trap 'rm -rf "$$work"' EXIT; \
	mkdir -p "$$work/raw" "$$work/bin"; \
	$(GO) test -covermode=atomic -coverpkg=./... -coverprofile="$$work/unit.out" ./...; \
	$(GO) build -cover -covermode=atomic -coverpkg=./... -o "$$work/bin/gozen" ./cmd/gozen; \
	(cd build/atago && PATH="$$work/bin:$$PATH" GOCOVERDIR="$$work/raw" $(ATAGO) run --ci --parallel 1 --filter capture --tag graphics ../../e2e/atago); \
	$(GO) tool covdata textfmt -i="$$work/raw" -o="$$work/cli.out"; \
	cat "$$work/unit.out" > coverage.out; \
	tail -n +2 "$$work/cli.out" >> coverage.out

lint: arch boundary tidy workflows
	$(GOLANGCI_LINT) run ./...
	$(GOLANGCI_LINT) fmt --diff
	cd examples/go-zen && $(GOLANGCI_LINT) run ./... && $(GOLANGCI_LINT) fmt --diff
	cd examples/novel && $(GOLANGCI_LINT) run ./... && $(GOLANGCI_LINT) fmt --diff
	cd examples/campaign && $(GOLANGCI_LINT) run ./... && $(GOLANGCI_LINT) fmt --diff

# go.mod and go.sum of every module match what the imports need.
tidy:
	$(GO) mod tidy -diff
	cd examples/go-zen && $(GO) mod tidy -diff
	cd examples/novel && $(GO) mod tidy -diff
	cd examples/campaign && $(GO) mod tidy -diff

# actionlint checks the syntax, expressions and shell of the workflows; zizmor
# checks their security (pins, credentials, injection, cache poisoning) with
# .github/zizmor.yml, offline so local and CI runs agree.
workflows:
	$(ACTIONLINT)
	$(ZIZMOR) --offline .github/workflows

arch:
	$(ARCH_LINT) check

boundary:
	$(GO) test ./internal/dev/hygiene -run TestApplicationPackagesAreInternal -count=1

# examples/go-zen is a nested module (it replaces gozen with this checkout), so
# ./... does not reach its integration tests. Games run through the root CLI.
# go-zen needs no display; go-zen-graphics takes its
# screens and needs one (CI runs it in the graphics job under xvfb-run).
go-zen:
	cd examples/go-zen && $(GO) vet ./...
	$(GO) run ./cmd/gozen check examples/go-zen/game
	cd examples/go-zen && $(GO) test -count=1 ./...

go-zen-graphics:
	cd examples/go-zen && $(GO) test -tags=graphics -count=1 ./...

# examples/novel, the novel sample, is a nested module too: novel checks and reads it
# headless, novel-graphics takes its screens (needs a display).
novel:
	cd examples/novel && $(GO) vet ./...
	$(GO) run ./cmd/gozen check examples/novel/game
	cd examples/novel && $(GO) test -count=1 ./...

novel-graphics:
	cd examples/novel && $(GO) test -tags=graphics -count=1 ./...

# examples/campaign, the campaign sample (ADR 0030), is a nested module too: campaign checks
# it and plays 200 campaigns headless, campaign-graphics takes its screens (needs a display).
campaign:
	cd examples/campaign && $(GO) vet ./...
	$(GO) run ./cmd/gozen check examples/campaign/game
	cd examples/campaign && $(GO) test -count=1 ./...

campaign-graphics:
	cd examples/campaign && $(GO) test -tags=graphics -count=1 ./...

# Requires a display; Linux CI wraps this target with xvfb-run.
graphics:
	$(GO) test -tags=graphics ./internal/play -run 'TestGraphicsScreens|TestGraphicsStage|TestGraphicsMap|TestGraphicsWild|TestGraphicsNovel|TestGraphicsCampaign|TestGraphicsSpace|TestGraphicsSimulateMatchesRun' -count=1
	$(GO) test -tags=graphics ./internal/layer5/puppet -run TestGraphics -count=1

cli-build:
	mkdir -p bin
	$(GO) build -trimpath -buildvcs=false -ldflags=-compressdwarf=false -o bin/gozen ./cmd/gozen

e2e: cli-build
	mkdir -p build/atago
	cd build/atago && PATH="$(CURDIR)/bin:$$PATH" $(ATAGO) run --ci --parallel 4 --skip-tag graphics,web ../../e2e/atago

desktop-runtime:
	mkdir -p build/runtime/local/THIRD_PARTY_LICENSES
	$(GO) build -trimpath -buildvcs=false -o build/runtime/local/gozen-player ./cmd/gozen-player
	$(GO) run ./internal/dev/runtimebundle -player build/runtime/local/gozen-player -target "$(shell $(GO) env GOOS)-$(shell $(GO) env GOARCH)" -version devel -out build/runtime/local/runtime.json
	cp LICENSE build/runtime/local/LICENSE
	GOROOT="$(shell $(GO) env GOROOT)" CGO_ENABLED=0 go-licenses save ./cmd/gozen-player --save_path=build/runtime/local/THIRD_PARTY_LICENSES --force

web-runtime:
	mkdir -p build/runtime/web
	GOOS=js GOARCH=wasm $(GO) build -trimpath -buildvcs=false -o build/runtime/web/gozen-player.wasm ./cmd/gozen-player
	$(GO) run ./internal/dev/runtimebundle -player build/runtime/web/gozen-player.wasm -target js-wasm -version devel -out build/runtime/web/runtime.json
	cp LICENSE build/runtime/web/LICENSE
	GOROOT="$(shell $(GO) env GOROOT)" CGO_ENABLED=0 GOOS=js GOARCH=wasm go-licenses save ./cmd/gozen-player --save_path=build/runtime/web/THIRD_PARTY_LICENSES --force

e2e-web: cli-build web-runtime
	mkdir -p build/atago
	cd build/atago && PATH="$(CURDIR)/bin:$$PATH" GOZEN_WEB_RUNTIME="$(CURDIR)/build/runtime/web" $(ATAGO) run --ci --parallel 1 --tag web ../../e2e/atago

e2e-graphics: cli-build desktop-runtime
	$(GO) build -trimpath -buildvcs=false -ldflags=-compressdwarf=false -o bin/gozen-game ./internal/play/testdata/host
	mkdir -p build/atago
	cd build/atago && PATH="$(CURDIR)/bin:$$PATH" GOZEN_GAME_BIN="$(CURDIR)/bin/gozen-game" GOZEN_RUNTIME="$(CURDIR)/build/runtime/local" GOZEN_EXE_SUFFIX="" $(ATAGO) run --ci --parallel 1 --tag graphics ../../e2e/atago

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
	$(MAKE) website-go-zen website-novel website-campaign

# All showcase games share the release player; only their YAML and assets differ.
# Pillow/WebP conversion is a site-maintenance optimization, not an author prerequisite.
website-player: web-runtime
	mkdir -p website/public/play/runtime
	cp -R build/runtime/web/wasm_exec.js build/runtime/web/LICENSE build/runtime/web/GO-LICENSE build/runtime/web/THIRD_PARTY_LICENSES website/public/play/runtime/
	gzip -9 -c build/runtime/web/gozen-player.wasm > website/public/play/runtime/gozen-player.wasm.gz

website-go-zen website-novel website-campaign: website-player
	mkdir -p website/public/play/$(patsubst website-%,%,$@)
	python3 scripts/web/assets.py examples/$(patsubst website-%,%,$@)/game build/web/$(patsubst website-%,%,$@)-game \
		scripts/web/index.html website/public/play/$(patsubst website-%,%,$@)/index.html build/runtime/web/gozen-player.wasm
	rm -rf website/public/play/$(patsubst website-%,%,$@)/game
	cp -R build/web/$(patsubst website-%,%,$@)-game website/public/play/$(patsubst website-%,%,$@)/game

# Opens the built Go Zen, novel and campaign pages in headless Chrome and fails when nothing is
# drawn, or when an idle Update or Draw after input/capture exceeds WEB_MAX_FRAME
# milliseconds (the page's frame meter, ?fps) (after make website; needs google-chrome and
# node).
WEB_MAX_FRAME ?= 250
website-play-check:
	node scripts/web_play_check.mjs -serve website/public /play/go-zen/ -enter 1 -shots build/web-play -max-frame $(WEB_MAX_FRAME)
	node scripts/web_play_check.mjs -serve website/public /play/novel/ -enter 2 -shots build/web-play/novel -max-frame $(WEB_MAX_FRAME)
	node scripts/web_play_check.mjs -serve website/public /play/campaign/ -enter 3 -shots build/web-play/campaign -max-frame $(WEB_MAX_FRAME)

website-serve:
	cd website && hugo server

# Broken internal links and pictures of the built site (after make website).
website-check:
	GOZEN_SITE_DIR="$(CURDIR)/website/public" $(GO) test ./internal/dev/docs -run '^TestSite(Links|Schema)$$' -count=1

# README and site pictures and steps (docs/development/docs.md). The pictures
# come from the samples listed in docs/images/manifest.yaml and need a display:
# on Linux they run inside xvfb-run, so no window opens on your screen.
# ONLY=a.jpg,b.gif (GOZEN_DOCS_IMAGES_ONLY) limits docs-images and docs-images-check to those pictures.
XVFB ?= $(shell command -v xvfb-run >/dev/null && echo 'xvfb-run -a -s "-screen 0 1920x1080x24"')
DOCS_TEST = $(GO) test ./internal/dev/docs -count=1 -timeout 40m

docs-images:
	GOZEN_DOCS_IMAGES=update $(if $(ONLY),GOZEN_DOCS_IMAGES_ONLY=$(ONLY)) $(XVFB) $(DOCS_TEST) -run '^TestDocImages$$' -v

docs-images-check:
	GOZEN_DOCS_IMAGES=check $(if $(ONLY),GOZEN_DOCS_IMAGES_ONLY=$(ONLY)) $(XVFB) $(DOCS_TEST) -run '^TestDocImages$$' -v

# Runs the shell steps of README.md and website/content against this checkout.
docs-steps:
	GOZEN_DOCS_STEPS=1 $(XVFB) $(DOCS_TEST) -run '^TestDocSteps$$'

docs: docs-images-check docs-steps website website-check
