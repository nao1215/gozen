# ADR 0033: Headless route exploration

Status: Accepted (2026-10-22).

## Users and problem

Authors of visual novels and event-driven games need to see whether authored choices lead to endings and whether conditional options are ever offered. Replaying by hand is easy to miss a branch, and this check should be usable in CI.

## Decision

Add `gozen sim routes <game-dir>` to the existing simulation command family. It uses `headless.Explore` with fixed deterministic gameplay randomness, a default cap of 300 explored paths, 20,000 requests per path, and a 30-second context deadline. The exploration context is checked between event requests. `--paths`, `--depth`, and `--timeout` tune those limits.

Text and JSON output report reached clear and game-over outcomes, their path counts and an example choice sequence, never-offered authored choice options, flags already diagnosed as set but never read (GZN0112), and whether exploration hit its path cap or timed out. `--fail-unreachable` opts into exit 1 when an ending, option, or flag is unreachable; otherwise these are findings, not command failures. Invalid arguments retain exit 2.

The command is a CLI concern in `cmd/gozen`; it does not add a public API or YAML fields. It starts with `game.start.event`, and `Explore` continues through automatic arrival events. Clear commands and the configured `game_over.event` are the recognized game outcomes. No scene-ending marker exists in the schema, so this command does not infer one from gallery scenes.

## Alternatives

1. Add a separate checker outside `sim`. Route walking is simulation without a display and fits the existing family.
2. Fail every unreachable finding by default. Conditional content can be intentional; CI users explicitly opt in.
3. Add an ending marker to YAML. Existing clear and game-over behavior already supplies stable outcomes, so a schema field is not needed.

## Consequences

- `api/public.txt` and the YAML schema inventory do not change.
- `headless.Config.Context` lets the CLI stop work on deadline or cancellation between event requests. The existing per-path request cap remains the secondary bound.
- JSON consumers can distinguish a capped report through `truncated`; a timed-out run may also include the cancellation error in `errors`.
- Flag findings reuse the existing static definition analysis in `DB.FlagWarnings`; the command does not add a separate dynamic flag tracker.
