# ADR 0023: Verify the images and steps of the README and the site

Status: Accepted. Date: 2026-10-20.

## Background

The README and the GitHub Pages site are the first thing users read. Images were taken by hand and placed, so stale images remained after the screens changed (`docs/images/puppet.gif` stayed old even after the sample's subtitles and layout changed). The code snippets of the steps were not run either: in the own-game steps of `getting-started`, `go run .` failed for lack of go.sum entries, `go list -m -f '{{.Dir}}' golang.org/x/image` came back empty in a fresh environment, and `gozen mosaic manual add` required a model. The site build also stopped for a lack of schema translation without anyone noticing.

## Decision

- The images shown by the README, the site and `docs/` are only those described in `docs/images/manifest.yaml` (the ledger), which records which sample is run with which arguments and script, which shot (or which frame chosen from a trace) is taken, and how it is encoded. `make docs-images` rebuilds all of them inside Xvfb. `go test` fails when an image not in the ledger is placed in `docs/images` or `website/`, and when the ledger and the references disagree (referenced but not in the ledger, in the ledger but not referenced, or the file is missing).
- Staleness is checked in CI by retaking the images with the same steps and comparing. Rendering jitter (software rendering, JPEG and GIF color reduction) is tolerated by the fraction of pixels whose per-pixel difference exceeds a threshold, and moving images are compared by frame count and five representative frames. An exact match is not required.
- The shell snippets of the README and `website/content` are run as they are. `go install`, `go run …@latest` and `go get` are not rewritten; they are resolved by a local module proxy that serves the local checkout as the latest version (which also checks that the published module zip would have the same content). Anything that cannot run is excluded with a mark such as `<!-- verify: skip reason -->`; an exclusion with no reason, and a YAML example with no mark, are failures. YAML examples are loaded with `gozen.Load`.
- The checks live in the tests of `internal/dev/docs`. The ledger, marks, YAML examples and README links run in the ordinary `go test`. Rebuilding, comparison, running the steps and the link check of the built site, which need a display or a build, are tests selected by environment variables (`make docs-images`, `docs-images-check`, `docs-steps`, `website-check`). They are tests so that no dependency exception is needed to let a development tool (`internal/dev`) load YAML and games (the same form as schema generation).

## Alternatives

- Do not commit images and only build them in CI: the README is shown by GitHub from the images in the repository, so committing is required.
- Compare by exact pixel match: it fluctuates with the Xvfb rendering and the ffmpeg version, giving meaningless failures.
- Write the steps with atago: the steps live in the documents and must be extracted from them and run in order, so they are Go tests. The CLI contract stays in `e2e/atago` as before.

## Result

When the sample is replaced (for example with Go Zen), only `samples` and `runs` in the ledger change, and the image names, references and checks stay as they are. The rules and how to write them are in [Images and steps in the README and the site](../development/docs.md). While the repository is private, the checks that need a display or a build follow the CI budget rule and run only after going public and on manual runs ([CI structure](../development/ci.md)).
