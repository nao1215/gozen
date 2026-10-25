# ADR 0043: Narrative authoring and executable YAML extensions

Status: Accepted.

## Authors and problem

Story authors should not define combat stats, basic attacks or a dummy party to
write dialogue, use portraits, make choices and save progress. Declared custom
commands and screens must have executable definitions rather than silently doing
nothing. Breaking changes to the unused declaration syntax are permitted.

## Decision

Keep character presentation in `actors`. A character without `class` is a speaker;
adding a class makes that same ID eligible for party membership and combat. Such
speakers retain faces, voices, staging and profiles without a runtime combat actor.
Games without combat actors, enemies, troops or a campaign permit an empty party,
including saved state, and do not require attack or guard skills. Games with combat
retain their stricter party and skill validation. New templates use the current
schema, and the novel template contains only narrative definitions.

Reusable story behavior stays in named events. Custom verb names resolve to these
events; they do not add another expression language or arbitrary Go callbacks.
Author-defined screens compose titled entries with text, optional pictures and
visibility conditions. The existing runtime owns their input and rendering.

`game.yaml.reactions` maps a speaker ID to a reaction book under assets. The
`respond` command submits authored/interpolated text to that speaker and presents
the resulting lines through the normal dialogue flow. Reaction memory is saved
with game state, and its random choices use the existing replayable game RNG.
Existing dialogue history is reused instead of introducing a second chat log.

Schema and validation remain in gamedata, progress and saves in their existing
owners, rendering in play, and packaging in command tooling. The `react` component is a lower-level expression-based rule interpreter: its
only engine dependencies are `expr` and foundation helpers. Game loading may
validate its books, and event execution may run its rules. These downward edges
replace its previously isolated status; `react` must not import game definitions,
state, events or presentation. No excluded file or public Go API is introduced. Trial selection must retain dependencies
of these definitions and report explicitly forbidden output before publication.

## Alternatives and impact

A separate speaker namespace would force authors to move and rename characters
when adding battles. Hidden combat defaults would keep the coupling and obscure
mistakes. Rejecting all extensions would remove useful author workflows; arbitrary
scripting would add unnecessary complexity. Existing battle games keep their
contracts. The old declaration-only extension syntax is replaced by executable
definitions and receives a validation error rather than a runtime no-op.

## Verification

Validate a classless speaker, reject that speaker as a party member, round-trip an
empty-party save, and exercise a generated novel with the real player. Check
extension references and cycles, screen content and visibility, and trial output.
Update schema, author instructions and CLI E2E together.
