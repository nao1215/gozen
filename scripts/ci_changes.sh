#!/usr/bin/env bash
# Decides whether a pull request changes anything beyond documentation, and
# writes code=true or code=false to $GITHUB_OUTPUT (docs/development/ci.md).
#
# The heavy jobs of the workflows wait for this answer instead of using
# on.paths, because a workflow that never starts leaves its required status
# check pending forever. A job skipped by `if` counts as passed.
#
# Documentation only means every changed file is one of: a Markdown file, the
# website, docs/ (not docs/examples, which tests read), a LICENSE file, or a
# GitHub issue template. Anything else, an unknown file, or a failure of this
# script gives code=true. Pushes, tags, schedules and manual runs always give
# code=true: only a pull request is judged. Run from the checkout of the merge
# commit of the pull request, with at least two commits of history.
set -u

event="${1:-${GITHUB_EVENT_NAME:-}}"
out="${GITHUB_OUTPUT:-/dev/stdout}"

emit() {
  echo "code=$1" >> "${out}"
  echo "code changed: $1"
  exit 0
}

[ "${event}" = pull_request ] || emit true

files="$(git diff --name-only HEAD^1 HEAD)" || emit true
[ -n "${files}" ] || emit true

while IFS= read -r f; do
  case "${f}" in
    "") ;;
    docs/examples/*) emit true ;;
    *.md | docs/* | website/* | LICENSE | LICENSE.* | LICENSE-* | .github/ISSUE_TEMPLATE/*) ;;
    *) emit true ;;
  esac
done <<< "${files}"

emit false
