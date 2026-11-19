#!/usr/bin/env bash
# Installs Ubuntu packages on a GitHub-hosted runner without waiting on a slow
# mirror (docs/development/ci.md). Usage: scripts/ci_apt_install.sh PACKAGE...
#
# The Azure mirror the runners prefer can keep transferring at a very low speed
# without timing out; one graphics shard spent 508 seconds on a step its sibling
# finished in 21. So this script prefers the official Ubuntu HTTPS archive
# (keeping the runner's other mirrors as fallbacks), lets apt retry a failed or
# stalled connection, and bounds the two network phases, the index update and
# the download, with a deadline. A phase that passes its deadline is started
# again, up to three attempts; apt resumes the partial downloads. The packages
# are unpacked only after every file is on disk, so the deadline never
# interrupts dpkg. A healthy run fetches the index and ffmpeg in about ten
# seconds each, so the deadlines are 60 and 90 seconds. Run it before any other
# step that uses apt, so that step reads the same mirror order.
set -euo pipefail

if [ "$#" -eq 0 ]; then
  echo "usage: $0 PACKAGE..." >&2
  exit 2
fi

if [ -f /etc/apt/apt-mirrors.txt ]; then
  sudo sed -i \
    -e '\|^http://azure\.archive\.ubuntu\.com/ubuntu/|s/priority:1/priority:2/' \
    -e '\|^https://archive\.ubuntu\.com/ubuntu/|s/priority:2/priority:1/' \
    /etc/apt/apt-mirrors.txt
fi

apt_options=(-o Acquire::Retries=3 -o Acquire::http::Timeout=30 -o Acquire::https::Timeout=30 -o DPkg::Lock::Timeout=60)
install=(install -y --no-install-recommends)

# APT_DEB_CACHE names a directory of .deb files that the workflow keeps with
# actions/cache, keyed by the runner image (so the files fit the packages it
# already has) and the package list. When it holds the packages, they are
# installed from it without the network; after a download, the files are copied
# into it for the workflow to save.
cache=${APT_DEB_CACHE:-}
if [ -n "$cache" ] && compgen -G "$cache/*.deb" > /dev/null; then
  if sudo apt-get "${apt_options[@]}" "${install[@]}" "$cache"/*.deb; then
    exit 0
  fi
  echo "the cached packages did not install; downloading them" >&2
fi

for attempt in 1 2 3; do
  if sudo timeout --kill-after=10 60 apt-get "${apt_options[@]}" update &&
    sudo timeout --kill-after=10 90 apt-get "${apt_options[@]}" "${install[@]}" --download-only "$@"; then
    # Packages the runner already has download nothing, and leave nothing to keep.
    if [ -n "$cache" ] && compgen -G "/var/cache/apt/archives/*.deb" > /dev/null; then
      mkdir -p "$cache"
      cp /var/cache/apt/archives/*.deb "$cache"/
    fi
    sudo apt-get "${apt_options[@]}" "${install[@]}" "$@"
    exit 0
  fi
  echo "apt attempt ${attempt} of 3 failed or passed its deadline" >&2
  sleep $((attempt * 10))
done
exit 1
