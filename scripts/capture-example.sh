#!/usr/bin/env bash
# Run under xvfb-run on Linux; see docs/examples/wayfinder.md#capture.
set -euo pipefail
cd "$(dirname "$0")/.."
for tool in go python3 ffmpeg ffprobe; do
    if ! command -v "$tool" >/dev/null; then
        echo "Missing $tool; see docs/examples/wayfinder.md#capture." >&2
        exit 1
    fi
done
mkdir -p build/capture docs/images
go build -trimpath -buildvcs=false -ldflags=-compressdwarf=false -o build/capture/wayfinder ./examples/play
capture_dir=$(mktemp -d)
trap 'rm -rf "$capture_dir"' EXIT
python3 scripts/capture-example.py "$capture_dir"
