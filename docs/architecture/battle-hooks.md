# Battle events and meters

A game author can name common event IDs in `battle.before` and `battle.after` of `game.yaml`. For why the public definitions were added, see [ADR 0013](../decisions/0013-battle-hooks-and-meter-limits.md).

## Order of execution

- `before` runs just before `event.Runner` returns a battle request. If the location changes during it, the battle and the rest of the original event are cancelled.
- A screen retry restores the state after the pre-processing. Redoing the same battle does not run the pre-processing again.
- `after` runs when the battle's result branch has advanced and reached the next choice or battle, or when the rest of the event has ended. If the post-processing changes the location, the pending choice or chain of battles is cancelled.
- On an ordinary defeat with no defeat branch, the wipe-out handling takes priority and `after` does not run. The progress of a game that omits the setting is unchanged.
- The `after_win` reservation scheme stays. When the original event ends, the reserved event runs first, and then the remaining `after` is handled. At the boundary of the next choice or battle `after` runs first, so do not depend on a fixed order between `after_win` and `after`.

Basis: `Answer` at `event/event.go:249`, `Next` at `event/event.go:298`, `begin` at `internal/play/play.go:1337`, `result` at `internal/play/battle.go:1107`. `headless/battle_hooks_test.go:15` and `internal/play/battle_hooks_test.go:12` check the common progress and the retry.

## Defeat recovery and casualty records

A `battle` command defaults to raising its defeated participants to 1 HP before
`on_lose`. Set `recover: false` to keep their final HP instead. This is independent
of `retry: false`, which only suppresses the retry question. Actors outside
`fighters` keep their HP. The same policy applies to rendered play, sessions,
headless exploration and authored event simulation.

```yaml
- battle: expedition_guard
  fighters: [guide]
  retry: false
  recover: false
  on_lose:
    - if: "last_battle.defeated.guide"
      then:
        - set: {flag.guide_lost: true}
        - leave: guide
    - say: "The expedition did not return."
```

Keep the protagonist in the party and send a different actor with `fighters` to
continue a campaign after an expedition loss. The troop and actor IDs above refer
to author definitions. No development-only HP assignments are needed.

Every completed battle replaces the read-only expression object `last_battle`:

| Expression | Meaning |
| --- | --- |
| `last_battle.result` | `"win"`, `"lose"` or `"escape"`; empty before any battle or for an unresolved standalone simulation |
| `last_battle.participated.<id>` | Whether the actor was in that battle's roster |
| `last_battle.hp.<id>` | Final HP before recovery or victory rewards; zero for an absent ID |
| `last_battle.defeated.<id>` | Present in that roster and final HP zero |
| `last_battle.casualties` | Number of participants with final HP zero |

Check participation when distinguishing an absent actor from a defeated one. The
record includes already-down actors in the selected roster. Healing, leaving the
party and saving do not change the record; the next battle replaces it. Victory
and escape also record partial casualties. A supplied session/headless outcome
records the participants' current HP; choosing a loss alone does not simulate
combat damage. A retry restores the pre-battle record and fighter selection.

Saves after result acknowledgement preserve the current HP and record without
replaying outcome callbacks. As with other battle boundaries, these saves resume
between events rather than resuming the rest of a suspended battle event.

## Meter limits

`meters[].max_value` is an expression that refers to the ordinary field state. A limit that evaluates successfully is rounded up to at least 1, and the value is clamped between 0 and the limit. When it is given, the current value and the limit are shown, and the `full` display when full still takes priority. If the expression is omitted, the fixed `max` is used, and if `max` is 0 the existing 100 is used. If evaluation fails, it also falls back to the fixed limit.

The names and syntax of the expression are checked at load time. Basis: `internal/layer2/gamedata/check.go:262`, `internal/play/meters.go:15`. Neutral usage examples are at `data/battle_hooks_test.go:10` and `internal/play/battle_hooks_test.go:44`.

## Authoring CLI

The battle curve of `gozen sim dive` reflects the pre- and post-processing in an exploration with event execution enabled. A return caused by resources is distinguished from defeat and lack of items, and the party returns to the same floor after the configured rest and resupply. A standalone win-rate measurement does not run the exploration's pre-processing. The HP consumption rate is taken before the return and recovery by post-processing. Basis: `rate`, `fight`, `end` and `returnAndResume` at `internal/headless/balance/balance.go:317`, `:335`, `:401` and `:431`.

The trial-version pruning takes `battle.before` and `battle.after` as roots and follows callees, assets and state changes. A tile's `after` points to another tile, so it is not treated as an event reference. Basis: `reach` at `cmd/internal/trial/prune.go:428`, `cmd/internal/trial/battle_hooks_test.go:11`.

The input path of memos keeps the existing contract selected by `--external-input` of `gozen run`. Normally input happens inside the screen. Even if an IME commit and ordinary characters arrive at the same time they are not added twice, and the Enter of a frame that consumed input is not used for saving. Connecting to a real OS IME needs a check per environment, and a passing unit test does not mean the input was confirmed on a real machine. Basis: `external-input` in `internal/play/entry.go`, `readInput` at `internal/play/memo.go:354`.
