# Production tools (developer notes)

`gozen prompt` and `gozen art mosaic` are the production-support subcommands of `gozen` ([ADR 0016](../decisions/0016-single-cli.md)). Installation, per-OS requirements (cgo, C compiler, ONNX Runtime), options, the catalog format, model fetching and detection setup are described for users on the site: [Production tools](https://nao1215.github.io/gozen/production-tools/). This page keeps only what matters when developing them.

The entry point is only the dispatch in `cmd/gozen/main.go`. `internal/promptgen/command.New` registers a fixed operation; the root dispatcher parses it and uses the common `cliio` output and exit contract. Mosaic still owns its dispatch until its migration. No public library API is added. Batch assembly of `gozen prompt --scenes` is in `internal/promptgen/command/batch.go`; the default class names of `gozen art mosaic` are in `internal/mosaic/settings/settings.go`.

## Synthetic inputs

The input, catalog and plan of the examples were newly made for this repository. The image is a 24×24 RGB gradient generated with the Python standard library, and there is no third-party material. The fixture for the WebP regression is made from the same gradient, and can be converted to lossless WebP with Pillow through the optional `--webp` argument (only regenerating needs `python3 -m pip install Pillow==12.1.1`). The repository's MIT license applies to the generator and the images.

```sh
python3 docs/examples/production/generate_fixture.py
# Copy pattern.png into a working game/assets/ and pass fixes.yaml.
go run ./cmd/gozen art mosaic auto --fix docs/examples/production/fixes.yaml --out processed game pattern.png
go run ./cmd/gozen art mosaic inspect --fix docs/examples/production/fixes.yaml --out processed game pattern.png
```

`fixes.yaml` processes only several rectangles without automatic detection, so it runs without a model or shared library. The runnable check, including how the original image is copied, is `e2e/atago/production.atago.yaml`.

## Verification

The normal `make test lint build` and `make e2e` cover every `gozen` subcommand and the internal implementations. A cgo-disabled build of everything does not count as verification of running inference. The `inference-build` job of CI separates the cgo build on 3 OSes and shows the reason for the inference tests when no model is given.

```sh
go test -v ./internal/mosaic/command -run TestONNX -count=1
```

To check real inference, set `ONNXRUNTIME_LIB` to the shared library, and set `GOZEN_MOSAIC_MODEL` for detection, `GOZEN_MOSAIC_SEG_MODEL` for segmentation and `GOZEN_MOSAIC_TAGGER_DIR` for tagging. Each test skips with a reason only when the model and library are not given. If they are given but cannot be read or do not fit, the test fails. The input is always newly synthesized pixels. Record the result of real inference together with the model, shared library and OS, and keep it distinct from skips caused by missing prerequisites.
