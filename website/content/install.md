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

`gozen` checks, simulates, documents and packages a game directory. It needs nothing else at run time: no cgo, no display, no sound device. The one exception is `gozen mosaic` with a detection model, which needs a cgo build; see [Production tools](/production-tools/).

To keep the command pinned with your game, add it as a Go tool in your module and run it with `go tool gozen`:

```shell
go get -tool github.com/nao1215/gozen/cmd/gozen@latest
go tool gozen check game
```

## The library

```shell
go get github.com/nao1215/gozen@latest
```

The rules, the headless driver and the YAML loader are pure Go and run anywhere Go does. Only the game window (the `play` and `runner` packages) needs the platform graphics libraries below.

## What the window needs

Gozen draws with [Ebitengine](https://ebitengine.org/) v2.10. A normal build needs no C compiler. The race detector needs cgo and a C compiler on every OS.

| OS | Prepare |
|:--|:--|
| Linux (Ubuntu 24.04 or later) | `sudo apt-get install libx11-6 libgl1 libglx-mesa0 libxcursor1 libxi6 libxinerama1 libxrandr2 libxrender1 libxext6 libasound2t64`. These are needed at run time even with `CGO_ENABLED=0`. |
| Windows | A GPU driver with OpenGL and a logged-in desktop. |
| macOS | A desktop session. For the race detector, `xcode-select --install`. |

Check the setup with the sample game:

```shell
go run github.com/nao1215/gozen/examples/play@latest --mute
```

Runtime checks are done on Linux. Windows and macOS are built in CI but have not been played yet; reports are welcome.

## Without a screen (CI, servers)

On Linux, run the game inside Xvfb, a virtual display. This takes a screenshot of the title screen and quits:

```shell
sudo apt-get install xvfb xauth
printf 'wait 60\nshot title\nquit\n' > check.txt
xvfb-run -a go run github.com/nao1215/gozen/examples/play@latest --dev --mute --script check.txt --shots shots
```

`shots/title.png` is the result. Scripted runs, traces and `gozen shots` work the same way inside Xvfb.

Tests that only use the rules (`headless`, `data`, `battle` and so on) need no display at all.

## Optional tools

| Tool | Used by |
|:--|:--|
| ffmpeg | `--record` to write an MP4 of a scripted or automatic play (`sudo apt-get install ffmpeg`, `brew install ffmpeg`, or the ffmpeg build's `bin` on `PATH` on Windows) |
| zenity | `--external-input` on Linux, for typing text in a system dialog instead of in the window |
| A C compiler and ONNX Runtime | `gozen mosaic` detection; see [Production tools](/production-tools/) |

No model, ONNX Runtime or asset download is needed to build or play a game.
