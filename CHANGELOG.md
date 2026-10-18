# Changelog

Changes before the first public release are tracked under Unreleased. All packages and four CLI commands share one module version. The v0 API is being established; experimental packages under `x/` carry no compatibility promise.

## Unreleased

### Added

- YAML game loading and validation, expressions, events, battle rules, world navigation and versioned saves.
- Headless execution, Ebitengine presentation, audio and an embedded-game runner.
- The rpgcheck, rpgtool, promptgen and mosaic commands; ONNX inference uses user-supplied models and runtime libraries.
- Original minimal, headless and graphical examples with asset source and license records.
- Architecture dependency checks, public API inventory checks and CLI contract tests using atago.
- CI for lint, unit tests across three operating systems and minimum/stable Go versions, graphics, CLI contracts, native and cross-builds, examples, race detection, govulncheck and CodeQL.
- Contribution and security reporting guidance, plus Dependabot updates for Go modules and GitHub Actions.

### Verification limits

- Published installation, Windows/macOS runtime checks and remote CI results remain unverified locally. Cross-builds do not establish runtime support.
- Real audio and ONNX inference need separate runtime verification; model-dependent test skips are not inference results.
