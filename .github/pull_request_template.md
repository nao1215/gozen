## Summary

Describe the problem and the intended behavior change.

Related issue: <!-- e.g. Closes #123, or Refs #123 if the reporter should confirm first -->

## Checklist

- [ ] Tests added or updated for the observable behavior
- [ ] `make test lint build` and `make e2e` pass locally (and `make graphics e2e-graphics` under Xvfb when screens changed)
- [ ] Public API changes explained, with the `api/public.txt` diff reviewed
- [ ] Docs updated when behavior changed (`README.md`, `docs/`, `website/`, and the command's `--help`)
- [ ] `CHANGELOG.md` updated under `## Unreleased`
- [ ] Cross-platform impact considered for Linux, macOS, and Windows
- [ ] No private game data, models or machine-specific paths
