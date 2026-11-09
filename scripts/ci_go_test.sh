#!/usr/bin/env bash
# Runs `go test ./...` for the unit job of the CI workflow (docs/development/ci.md).
#
# On Windows, the Go 1.27 runtime sometimes corrupts its own heap in pure Go
# test binaries: the garbage collector stops with "fatal error: found pointer to
# free object", "fatal error: found bad pointer in Go heap" or "fatal error:
# index out of range" in its marking code, or the process dies with
# "Exception 0xc0000005". See golang/go#81238 (exception recovery writes below a
# goroutine stack on windows/amd64) and golang/go#82394. internal/mosaic and
# internal/promptgen have crashed this way without unsafe code, cgo or races.
#
# Only that crash is retried, and only on Windows: a failed package is run
# again, up to three more times, when it is listed below, its output has one of
# the runtime signatures, and it reports no failed test and no panic. Any other
# failure, any other package, or a build failure fails at once. Remove this
# script when the upstream issue is fixed in the Go versions the job tests.
set -u

go="${GO:-go}" # The test of this script substitutes a stand-in.

retryable=" github.com/nao1215/gozen/internal/mosaic github.com/nao1215/gozen/internal/promptgen "
attempts=3

log="$(mktemp)"
trap 'rm -f "${log}"' EXIT

# run_tests PACKAGES... runs go test, streaming its output and keeping a copy.
run_tests() {
  "${go}" test "$@" | tee "${log}"
  return "${PIPESTATUS[0]}"
}

# crashed_packages prints the failed packages when every failure is the known
# runtime crash of a listed package, and fails otherwise.
crashed_packages() {
  awk -v retryable="${retryable}" '
    function reset() { crash = 0; failed = 0 }
    BEGIN { reset(); bad = 0; n = 0 }
    /^fatal error: (found pointer to free object|found bad pointer in Go heap|index out of range)$/ || /^Exception 0xc0000005 / { crash = 1; next }
    /^--- FAIL: / || /^panic: / { failed = 1; next }
    /^(ok  |\?   )\t/ { reset(); next }
    /^FAIL\t/ {
      split($0, f, "\t")
      if (f[3] == "" || crash == 0 || failed == 1 || index(retryable, " " f[2] " ") == 0) bad = 1
      else list[n++] = f[2]
      reset(); next
    }
    /^FAIL$/ { next }
    END {
      if (bad || n == 0) exit 1
      for (i = 0; i < n; i++) print list[i]
    }
  ' "${log}"
}

run_tests ./...
status=$?
[ "${status}" -eq 0 ] && exit 0
[ "$("${go}" env GOOS)" = windows ] || exit "${status}"

for attempt in $(seq "${attempts}"); do
  packages="$(crashed_packages)" || exit "${status}"
  # Word splitting is intended: import paths contain no spaces.
  # shellcheck disable=SC2086
  echo "::warning::Go runtime crash on Windows (golang/go#81238), retry ${attempt}/${attempts}:" ${packages}
  # shellcheck disable=SC2086
  run_tests -count=1 ${packages}
  status=$?
  [ "${status}" -eq 0 ] && exit 0
done
exit "${status}"
