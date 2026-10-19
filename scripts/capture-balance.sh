#!/usr/bin/env bash
# Run under xvfb-run on Linux; see docs/development/graphics.md#バランス表示.
# Writes docs/images/wayfinder-balance.jpg (960x540, the same encoding as capture-example).
set -euo pipefail
cd "$(dirname "$0")/.."
for tool in go ffmpeg; do
    if ! command -v "$tool" >/dev/null; then
        echo "Missing $tool; see docs/development/graphics.md." >&2
        exit 1
    fi
done
mkdir -p build/capture docs/images
go build -trimpath -buildvcs=false -ldflags=-compressdwarf=false -o build/capture/wayfinder ./examples/play
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
timeout 240 build/capture/wayfinder --dev --mute --seed 7 --window 1 --start grotto \
    --save-dir "$work/saves" --script scripts/capture-balance.txt --shots "$work"
ffmpeg -hide_banner -loglevel error -y -i "$work/balance.png" -q:v 3 docs/images/wayfinder-balance.jpg
echo docs/images/wayfinder-balance.jpg
