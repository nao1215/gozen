#!/usr/bin/env bash
# Smoke-test the gozen release artifacts that GoReleaser wrote to DIST (default
# dist/). On Linux it checks every archive and Linux package for the binary, the
# notices and the bundled dependency licenses, verifies checksums.txt, and runs
# the Linux binary of this machine's architecture. On macOS it runs the macOS
# binary of this machine's architecture. Windows is checked by release.yml with
# PowerShell. Used by .github/workflows/release.yml; locally run it after
# `goreleaser release --snapshot --clean --skip=publish,sign`.
set -euo pipefail
cd "$(dirname "$0")/.."
dist=${1:-dist}
fixture=e2e/atago/fixtures/game

fail() {
    echo "smoke: $*" >&2
    exit 1
}

case "$(uname -m)" in
    x86_64 | amd64) arch=amd64 ;;
    arm64 | aarch64) arch=arm64 ;;
    *) fail "unsupported machine $(uname -m)" ;;
esac

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# run_binary checks that an unpacked gozen prints a version and checks the
# neutral fixture game.
run_binary() {
    local bin=$1
    local version
    version=$("$bin" version)
    case "$version" in
        "gozen v"*) ;;
        *) fail "$bin version printed '$version'" ;;
    esac
    "$bin" check "$fixture" > "$work/check.txt" || fail "$bin check $fixture failed"
    grep -q '^ok: ' "$work/check.txt" || fail "$bin check printed no ok line"
    echo "smoke: $version runs and checks $fixture"
}

case "$(uname -s)" in
    Linux)
        (cd "$dist" && sha256sum --check --quiet checksums.txt) || fail "checksums.txt does not match"
        for os in linux darwin windows; do
            for a in amd64 arm64; do
                if [ "$os" = windows ]; then
                    archive=$(ls "$dist"/gozen_*_"$os"_"$a".zip)
                    list=$(unzip -Z1 "$archive")
                    binary=gozen.exe
                else
                    archive=$(ls "$dist"/gozen_*_"$os"_"$a".tar.gz)
                    list=$(tar -tzf "$archive")
                    binary=gozen
                fi
                for want in "$binary" LICENSE README.md CHANGELOG.md ASSETS.md \
                    THIRD_PARTY_LICENSES/github.com/hajimehoshi/ebiten/v2/LICENSE \
                    THIRD_PARTY_LICENSES/github.com/goccy/go-yaml/LICENSE; do
                    grep -qx "$want" <<< "$list" || fail "$archive has no $want"
                done
            done
        done
        for pkg in "$dist"/gozen_*_linux_*.deb; do
            # Captured first: grep -q stopping early would fail the pipe under pipefail.
            files=$(dpkg-deb -c "$pkg")
            grep -q '/usr/share/doc/gozen/THIRD_PARTY_LICENSES/' <<< "$files" ||
                fail "$pkg has no THIRD_PARTY_LICENSES"
        done
        if command -v rpm > /dev/null; then
            for pkg in "$dist"/gozen_*_linux_*.rpm; do
                files=$(rpm -qlp "$pkg")
                grep -q '/usr/share/doc/gozen/THIRD_PARTY_LICENSES/' <<< "$files" ||
                    fail "$pkg has no THIRD_PARTY_LICENSES"
            done
        fi
        tar -xzf "$(ls "$dist"/gozen_*_linux_"$arch".tar.gz)" -C "$work"
        run_binary "$work/gozen"
        ;;
    Darwin)
        tar -xzf "$(ls "$dist"/gozen_*_darwin_"$arch".tar.gz)" -C "$work"
        run_binary "$work/gozen"
        ;;
    *)
        fail "run this script on Linux or macOS"
        ;;
esac
