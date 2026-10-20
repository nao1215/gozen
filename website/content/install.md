---
title: Install
description: Install the gozen command and the Gozen library, and prepare Linux, Windows, or macOS to open the game window.
weight: 10
---

Gozen needs Go 1.26.9 or later. The `gozen` command and the library are one Go module.

## The gozen command

```shell
go install github.com/nao1215/gozen/cmd/gozen@latest
gozen version
```

`gozen` checks, simulates, documents and packages a game directory. It needs nothing else at run time: no cgo, no display, no sound device. The one exception is `gozen art mosaic` with a detection model, which needs a cgo build; see [Production tools](/production-tools/).

From the first release on, `gozen` is also published as prebuilt binaries. Gozen has no release yet, so these channels start with it:

<!-- verify: skip the Homebrew cask is published with the first release -->
```shell
brew install --cask nao1215/tap/gozen   # macOS and Linux with Homebrew
```

[The release page](https://github.com/nao1215/gozen/releases) has `.tar.gz` and `.zip` archives for Linux, macOS and Windows (amd64 and arm64) and `.deb`, `.rpm` and `.apk` packages for Linux. The prebuilt binaries are built without cgo, so they have no ONNX inference for `gozen art mosaic`. Each release signs `checksums.txt` with cosign and attaches SBOMs and SLSA provenance; [Verifying a release](#verifying-a-release) shows the commands.

To keep the command pinned with your game, add it as a Go tool in your module and run it with `go tool gozen`:

<!-- verify: run fixture=module -->
```shell
go get -tool github.com/nao1215/gozen/cmd/gozen@latest
go tool gozen check game
```

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

## The library

<!-- verify: run fixture=module -->
```shell
go get github.com/nao1215/gozen@latest
```

The rules, the headless driver and the YAML loader are pure Go and run anywhere Go does. Only the game window (the `play` package) needs the platform graphics libraries below.

## What the window needs

Gozen draws with [Ebitengine](https://ebitengine.org/) v2.10. A normal build needs no C compiler. The race detector needs cgo and a C compiler on every OS.

| OS | Prepare |
|:--|:--|
| Linux (Ubuntu 24.04 or later) | `sudo apt-get install libx11-6 libgl1 libglx-mesa0 libxcursor1 libxi6 libxinerama1 libxrandr2 libxrender1 libxext6 libasound2t64`. These are needed at run time even with `CGO_ENABLED=0`. |
| Windows | A GPU driver with OpenGL and a logged-in desktop. |
| macOS | A desktop session. For the race detector, `xcode-select --install`. |

Check the setup with a small window, a moving portrait:

<!-- verify: build opens the game window and waits for a player -->
```shell
go run github.com/nao1215/gozen/examples/puppet@latest
```

Runtime checks are done on Linux. Windows and macOS are built in CI but have not been played yet; reports are welcome.

## Without a screen (CI, servers)

On Linux, run the window inside Xvfb, a virtual display. This plays the portrait's motions once, writes the frames and quits:

<!-- verify: run except=1 installs system packages with sudo; the check runs inside Xvfb -->
```shell
sudo apt-get install xvfb xauth
xvfb-run -a go run github.com/nao1215/gozen/examples/puppet@latest --tour --frames frames
```

`frames/` has the pictures. In your own game, `--dev --script FILE --shots DIR` takes screenshots the same way. Scripted runs, traces and `gozen play shots` work the same way inside Xvfb.

Tests that only use the rules (`gozen.Session` and `gozen.Explore`) need no display at all.

## Optional tools

| Tool | Used by |
|:--|:--|
| ffmpeg | `--record` to write an MP4 of a scripted or automatic play (`sudo apt-get install ffmpeg`, `brew install ffmpeg`, or the ffmpeg build's `bin` on `PATH` on Windows) |
| zenity | `--external-input` on Linux, for typing text in a system dialog instead of in the window |
| A C compiler and ONNX Runtime | `gozen art mosaic` detection; see [Production tools](/production-tools/) |

No model, ONNX Runtime or asset download is needed to build or play a game.
