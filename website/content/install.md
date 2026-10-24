---
title: Install
description: Install the Gozen binary without a programming toolchain, and prepare your desktop for playing games.
weight: 10
---

To create and play games, install the prebuilt `gozen` binary for your operating system. No Go compiler is required. Go 1.26.10 or later is needed only when building the engine or using its transitional Go API.

## The gozen command

Download the archive for your OS and processor from [Releases](https://github.com/nao1215/gozen/releases), extract it, and put its executable in a directory on your PATH. Keep the license notices from the archive. You can also install it with Homebrew:

<!-- verify: skip installs from the Homebrew tap, outside the repository -->
```shell
brew install --cask nao1215/tap/gozen   # macOS and Linux with Homebrew
```

[The release page](https://github.com/nao1215/gozen/releases) has `.tar.gz` and `.zip` archives for Linux, macOS and Windows (amd64 and arm64) and `.deb`, `.rpm` and `.apk` packages for Linux. The prebuilt binaries are built without cgo, so they have no ONNX inference for `gozen art mosaic`. Each release signs `checksums.txt` with cosign and attaches SBOMs and SLSA provenance; [Verifying a release](#verifying-a-release) shows the commands.

After installation, `gozen version` reports the installed version. Follow [Getting started](/getting-started/) to create your first game. `new`, `check` and simulation do not need a display; `play run` needs the graphics runtime described below. Inference tools have additional prerequisites described in [Production tools](/production-tools/).

## Optional installation from Go source

These steps are for engine developers and existing Go projects. They are not needed for YAML game creation.

```shell
go install github.com/nao1215/gozen/cmd/gozen@latest
gozen version
```

To keep the command pinned with your game, add it as a Go tool in your module and run it with `go tool gozen`:

<!-- verify: run fixture=module -->
```shell
go get -tool github.com/nao1215/gozen/cmd/gozen@latest
go tool gozen check game
```

## In GitHub Actions

[setup-gozen](https://github.com/nao1215/setup-gozen) installs the release binary of the version in your `go.mod`, checked against the release checksums:

<!-- verify: skip runs in GitHub Actions -->
```yaml
- uses: actions/checkout@v6
- uses: nao1215/setup-gozen@1c378fc446f62ecb29a7bcd40aea6dff8565d698 # v0.1.1
- run: gozen check game
```

`version` also takes `latest` or a release such as `v0.2.0`, and `go-mod-path` points at a `go.mod` below the root. Without the action, `go run github.com/nao1215/gozen/cmd/gozen check game` builds the same version from `go.mod`. See [the CI recipe](/cookbook/#check-and-measure-in-ci).

## Verifying a release

Each release has a cosign-signed `checksums.txt`, an SPDX SBOM per archive, SLSA provenance (`multiple.intoto.jsonl`), and the dependency licenses under `THIRD_PARTY_LICENSES/`.

<!-- verify: skip needs the files of a published release -->
```shell
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp 'https://github.com/nao1215/gozen/\.github/workflows/release\.yml@refs/tags/.*' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
  checksums.txt
sha256sum --check --ignore-missing checksums.txt
```

<!-- verify: skip needs a published release and a GitHub login -->
```shell
gh attestation verify gozen_<version>_<os>_<arch>.tar.gz --repo nao1215/gozen
```

<!-- verify: skip needs the files of a published release -->
```shell
slsa-verifier verify-artifact gozen_<version>_<os>_<arch>.tar.gz \
  --provenance-path multiple.intoto.jsonl \
  --source-uri github.com/nao1215/gozen \
  --source-tag v<version>
```

## The library during migration

<!-- verify: run fixture=module -->
```shell
go get github.com/nao1215/gozen@latest
```

The public Go API remains available during the no-code migration; it is not part of the beginner workflow. The rules, the headless driver and the YAML loader are pure Go and run anywhere Go does. Only the game window (the `play` package) needs the platform graphics libraries below.

## What the window needs

Gozen draws with [Ebitengine](https://ebitengine.org/) v2.10. A normal build needs no C compiler. The race detector needs cgo and a C compiler on every OS.

| OS | Prepare |
|:--|:--|
| Linux (Ubuntu 24.04 or later) | `sudo apt-get install libx11-6 libgl1 libglx-mesa0 libxcursor1 libxi6 libxinerama1 libxrandr2 libxrender1 libxext6 libasound2t64`. These are needed at run time even with `CGO_ENABLED=0`. |
| Windows | A GPU driver with OpenGL and a logged-in desktop. |
| macOS | A desktop session. For the race detector, `xcode-select --install`. |

Check the setup by opening a generated game:

```shell
gozen new window-check
```

<!-- verify: skip opens a window and waits for a player; graphics E2E plays generated games -->
```shell
gozen play run window-check
```

Runtime checks are done on Linux. Windows and macOS are built in CI but have not been played yet; reports are welcome.

## Without a screen (CI, servers)

On Linux, run the window inside Xvfb, a virtual display. This creates a game,
captures its title screen and quits:

<!-- verify: run except=1 installs system packages with sudo; the check runs inside Xvfb -->
```shell
sudo apt-get install xvfb xauth
gozen new ci-check
printf 'wait 20\nshot title\nquit\n' > check.txt
xvfb-run -a gozen play run ci-check --dev --mute --script check.txt --shots frames
```

`frames/title.png` contains the title screen. Scripted runs, traces and screenshot
capture use the same runner as ordinary play. `gozen check` and `gozen sim` need no
display at all.

## Optional tools

| Tool | Used by |
|:--|:--|
| ffmpeg | `--record` to write an MP4 of a scripted or automatic play (`sudo apt-get install ffmpeg`, `brew install ffmpeg`, or the ffmpeg build's `bin` on `PATH` on Windows) |
| zenity | `--external-input` on Linux, for typing text in a system dialog instead of in the window |
| A C compiler and ONNX Runtime | `gozen art mosaic` detection; see [Production tools](/production-tools/) |

No model, ONNX Runtime or asset download is needed to build or play a game.
