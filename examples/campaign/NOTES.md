# Five Banners: notes

Five Banners is a small region-conquest campaign. Stage 0 wrote it with the YAML Gozen had
(schema version 4) to find where such a game is hard to write; stage 2 rewrote it with the
campaign words of [ADR 0030](../../docs/decisions/0030-campaigns.md) (schema version 5); stages
7, 8 and 8b made it Kichikuou-leaning (weeks, an order once a turn, generals away after a
rout, a sortie screen, clashes of units with tactics and skills, region areas). The text is
English with a Japanese translation (`lang/ja.yaml`).

## Stage 2: the campaign words

The game is the same: five regions, the Crown against the Raven and the Stag, ten turns,
three orders, four generals (Corin, Hilde, Sable who joins by a talk at turn 3, Vessa who
joins when the Crown takes Crossway), a truce offered at turn 6. The generals' pictures are
now the author's own (see ASSETS.md). What changed:

- `data/war.yaml` defines `regions` (names, borders, owners, garrisons, wealth) and
  `factions` (colour, capital, and the Raven's and the Stag's rules: draft 190 into every
  region, then march once from a region of at least 1000 along the border whose `where`
  holds, the weakest first). `game.yaml`'s `campaign` names the place, the player, the
  turns, the actions, the generals, the orders and the clash.
- `rules.yaml` is gone: no variables to declare (the state has generated names), no dispatch
  events (`target` and `general` are bound by the order), no tally (`faction.crown.regions`),
  no hand-written AI, no `clear` for a defeat (losing the capital is a game over), and the
  income is one line (`gold: faction.crown.sum_wealth`).
- The harness gives orders as `- order: {id: attack, general: hilde, target: ford}` and reads
  `region`, `generals` and `campaign` as whole objects with `Query`, instead of building
  `var.tr_<id>` names and matching option labels.

| Part | Stage 0 lines | Stage 2 lines |
|:--|:--|:--|
| `game.yaml` | 92 | 48 |
| `data/rules.yaml` | 199 | (gone) |
| `data/war.yaml` | 78 | 51 |
| `data/story.yaml` | 46 | 58 (the won and lost events moved here, and the income) |
| `data/people.yaml` | 14 | 14 |
| YAML in all | 429 | 171 (-60%) |
| `campaign_test.go` | 475 | 344 |

Measured with `go test -v` (Seed 1 to 200 for each strategy):

| Strategy | Wins | Median turn of a win | Regions held at the end |
|:--|:--|:--|:--|
| greedy | 124 (62%) | 10 | 1:1 2:1 3:57 4:17 5:124 |
| random | 0 (0%) | - | 0:81 1:86 2:32 3:1 |

- The same seed gives the same end. A save at the start of turn 5, restored with another
  seed, now ends exactly as the campaign saved (20 of 20): a save between commands keeps the
  random numbers.
- Picking Attack from the menu asks "Who leads?" and "Where to?" with Cancel last; the
  Policy answers them. Explore still finds both answers to Sable.
- Of the stage 0 mistakes `gozen check` missed, these are now errors: an owner that is not a
  faction (in `regions`, `capital`, and a `set` of a literal owner), a border written on one
  side. The border of an order and of a faction's march is no longer written by hand. Still
  not caught: a misspelt faction compared in an expression (`region.vale.owner == 'crwon'`),
  an event that calls itself.
- The balance is steep: the AIs' draft moved greedy play from 89% (180) to 62% (190), and
  `keep` from 92% (1500) to 0% (1600). The numbers of a faction's rules need a sweep in a
  `sim campaign`, not by hand.

Still for a later stage: the map draws no faction colours, no turn band and no meters (the
two plates on roads that never open are still there); the orders are in the Menu list, not on
the map; there is no `gozen sim campaign` (the harness is still Go); a battle cannot decide a
march.

## Stage 0: today's words

Stage 0 wrote the game without changing the engine, to find where such a game is hard to
write, as the ground for the YAML words of stage 1 (`regions`, `factions`, `campaign`). Its
generals were Go Zen's (Maren, Ox, Tilly, Kes).

### What it was

- Five regions (Ashvale, Brookford, Crossway, Highhill, Deepwell) on Go Zen's town map,
  joined by six borders. Three factions: the Crown (the player, capital Ashvale), the Raven
  and the Stag (AI, one region each), and two free regions.
- Ten turns. Each turn the Crown has `2 + floor(regions / 2)` orders: Attack (a general
  against a region next to the Crown), Develop (wealth +15), Draft (150 troops for 50 gold).
  End turn lets the Raven and the Stag move, pays the wealth of the Crown's regions as gold
  and starts the next turn.
- Four generals (Maren, Ox, Tilly, Kes); Tilly joins by a talk at the start of turn 3 (a
  choice) and Kes when the Crown takes Crossway. A Raven envoy offers a truce at turn 6.
- Won when the Crown holds all five regions; lost when Ashvale falls or turn 10 ends.
- The screen is the map (`map:`): each region's plate shows `{name} {owner} {troops}`; two
  extra plates show the turn, orders, gold and news, and the generals' troops. The orders are
  the location's actions, in the list (Menu).

### Measured

`go test -v` in this directory (`campaign_test.go`, a harness of `gozen.New`,
`Config{Seed, Policy}`, `Session.Do("- pick: ...")` and `Session.Query`):

| Strategy | Wins of 200 seeds | Median turn of a win | Regions held at the end |
|:--|:--|:--|:--|
| greedy (march when the odds are 1.15 or better, else draft, else develop) | 95 (48%) | 6 | 0:17 1:7 2:78 3:2 4:1 5:95 |
| random (any order the menu offers) | 0 (0%) | - | 0:23 1:78 2:88 3:11 |

- The same seed gives the same end (20 seeds).
- Save at the start of turn 5 and Restore: every declared name, gold, party and location are
  the same. Of 20 campaigns played on, 14 end the same after Restore and 6 do not; a Clone
  ends the same every time. The cause is the random numbers (see below).
- `gozen.Explore` from the end of turn 2 finds both answers to Tilly, and only taking her in
  sets `flag.has_tilly`.
- `gozen check` passes. Screenshots under Xvfb (`--script`) show the map, the order list, the
  choices and the plates changing after a march.

The first balance had the AI march 60% of any garrison along the first border that it could
beat. The AIs emptied their capitals fighting each other over Crossway, and greedy play won
200 of 200 by turn 4. A rule "march only from a region of at least `var.keep` (1000)
troops", AI drafts of 170 a region, and dearer drafts for the Crown brought it to 48%.

### Size

| Part | Lines | Notes |
|:--|:--|:--|
| `game.yaml` | 92 | 61 declared state names: 4 per region, 3 to 4 per general, 20 for the turn and the arguments of shared events |
| `data/war.yaml` (map and orders) | 78 | 7 points, 8 roads, 4 actions with 3 choices of 5 to 6 options |
| `data/rules.yaml` | 199 | 13 events, 39 `if`s |
| `data/story.yaml` | 46 | 4 events |
| `data/people.yaml` | 14 | a class, 4 actors, the two required skills |
| `campaign_test.go` | about 470 | the measuring harness: strategies, a Policy, reading the state |

Of `rules.yaml`, the events that only move values between "the region named in `var.r`" and
its variables (`get_region`, `put_region`, `get_general`, `put_general`) are 52 lines,
the AI (`ai_turn`, `ai_clash`, `ai_draft`) 68 lines, and counting regions (`tally`) and the
income are 3 expressions of 5 terms each, one per faction.

### Where it was hard

#### Writing the rules

1. No parameters and no tables. A shared event takes its arguments in variables (`var.g`,
   `var.t`, `var.r`, `var.f`), and every write to "the region in `var.r`" is a dispatch, one
   `if` per region, because the left side of `set` is written as is. Adding a sixth region
   touches about twelve places (four state names, its plate, the Attack and Develop options,
   `get_region`, `put_region`, three terms of `tally`, the income, `ai_draft`, and every AI
   border line that involves it).
2. Strict state names and reading by a built name do not mix. `var['tr_' + var.t]` reads in a
   game without `state.strict`, but strict refuses it ("var must name a declared name
   directly", GZN1103). The choice is typo checks or one-line reads; this prototype kept
   strict and wrote the reads as dispatches too (`get_region`, `get_general`).
3. Constants live in variables. A region's display name and a general's might have no other
   home (`var.nm_vale`, `var.mt_ox`), so they are state that a save carries and that `set`
   could change.
4. A faction is a string. Owners are the strings `'Crown'`, `'Raven'`, `'Stag'`, `'Free'`; a
   typo such as `'Crwon'` passes `gozen check`.
5. The borders are written three times: as the map's roads, in the `when` of each Attack
   option, and as the AI's twelve directed border lines. Nothing checks that they agree
   (the Go harness repeats them a fourth time).
6. Counting is spelled out: "how many regions the Stag holds" is five ternaries; "the region
   next to the Crown with the fewest troops" cannot be written at all.
7. There is no defeat without a battle. The only game over is a lost battle, so both ends
   use `clear` with `flag.won` or `flag.lost`; the player sees the same ending screen, and
   `Query("over")` is false after a defeat.
8. YAML trap: an unquoted line with a comma inside a flow map (`{text: Five banners, one
   colour.}`) became an unknown field. `gozen check` caught it, with the message `unknown
   field "one colour. It is over."` (GZN1104), which does not point at the comma.

#### Writing the AI

- The AI is an event per turn with one `if` per directed border (12 here, 2 per border),
  taken in the order written: the order of the lines is the AI's preference. There is no
  `pick: min` over regions, so "attack the weakest neighbour" or "defend the threatened
  region" needs a dispatch per pair.
- Because the AI is ordinary events, a bad rule shows only in the numbers (greedy 100%
  above). Nothing reports "the AI never attacks the Crown" or "the AI empties its capital".

#### Mistakes that `gozen check` did not catch

Each was planted in a copy of the game; only the last was reported:

| Mistake | Caught |
|:--|:--|
| An owner string misspelt (`'Crwon'`) | no |
| A border missing from an Attack option's `when` | no |
| An AI border line between regions that do not touch | no |
| A region missing from the dispatch `put_region` (writes to it are lost) | no |
| An event that calls itself (stopped at run time by the 20000-step limit) | no |
| A turn limit the campaign can never reach (`var.turn > 100`) | no |
| An undeclared variable (strict) | yes, with file and line |

The list of known names in that last message also holds the dungeon's generated names
(`deepest`, `dive_seed`, `floor_left`, `roam_calm` ...), though this game has no dungeon.

#### The screen

- The map screen draws no meters and no `desc`. The turn, orders, gold and news, and the
  generals' troops, are shown as the plates of two extra points (`hq`, `camp`).
- Every point needs a road (`gozen check`: "no road leads to the point"), so the two plates
  get roads that never open (`when: "false"`), and they stand pins like the regions.
- Every marker is the same pin: the owner can only be read from the plate's text, not from a
  colour.
- The orders are hidden behind Menu, and choosing a region on the map is not an order: it
  walks the token there. The point the token stands on cannot be chosen.
- A choice's window covers the lower plates (the turn and the generals) while it is open.

#### Measuring

- There is no `gozen sim` for a campaign, so the numbers above come from a Go test: about 470
  lines that repeat the borders and the odds of a march, read the state name by name, and
  answer choices by the prefix of an option's label ("Maren (", "Highhill ("). A renamed
  label breaks the strategy silently (it falls back to Cancel).
- `Explore` branches on choices, not on `pick`, so it covers the talk (Tilly, the truce), not
  the orders.
- A save made between commands does not keep the random numbers: `Restore` draws from
  `Config.Seed` again (`rand(1000000)` right after Restore differs from the original). With
  a roll in every march, 6 of 20 campaigns ended differently after a save and a restore at
  turn 5. Saving and loading to roll a march again is possible, and a trace that crosses a
  restore cannot be replayed exactly.

### What this said about the stage 1 words

- `regions.<id>: {name, at, next, owner, troops, values}` — confirmed. It removes the four
  state names per region, the name constants, the dispatches, and the three copies of the
  borders; check can then compare `next` with the roads and owners with the factions.
- `factions.<id>: {name, color, capital, ai}` — confirmed. `color` is needed on the map (the
  pins), and owner must be a reference, not a string. `faction.<id>.regions` replaces `tally`.
- `campaign: {turns, actions, win, lose, over, on_turn}` — confirmed, and `over` should be a
  real defeat (game over), not `clear`. The turn and orders band belongs to the screen,
  since the map screen has none today.
- Orders with a bound region and general (`region`, `generals` in `do`) — the most needed
  change: binding is what the dispatch events emulate. Without it, every per-region write is
  `regions × writes` lines.
- The AI as rules with `target: {where, pick, by}` — confirmed; first-fit by line order was
  too weak to write a sensible AI, and a minimum garrison (`keep`) had to be added by hand.
  The rule table should have a way to say "keep N at home" (a `where` on the source region).
- Add: the random numbers in every save (or at least in a campaign's save), so that a
  Restore plays on the same; and `gozen sim campaign` with a named strategy, so the numbers
  above come from the CLI's JSON instead of a Go harness.
- Add to check: owner and capital references, symmetric `next`, orders and AI rules that name
  unknown orders, a `win` that holds at the start, and a turn limit beyond `turns`.
- Not needed yet: the battle bridge (`fight: battle`). The numeric clash was enough to make
  a playable 10-turn game; whether a battle per march is fun is a later question.
