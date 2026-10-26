# Monster collection

Use schema version 6. Define `monster_families`, then give each non-boss enemy a `family`, `rank` and `monster_class`; use `recruit: {chance, level}` for species that may ask to join after a won battle (`admission: party_only` keeps recruits out of the reserve; the `capture` command recruits directly, see battle.md). The class supplies growth, experience and skills learned by level.

Put `breeding: {min_level, skill_slots, pairs, families}` in data. Set `enemies.<id>.sex` to `any` (default), `male` or `female`; optional `sex_ratio` is the male probability for `any`. Captured and born instances keep their assigned sex. Breeding needs one male and one female, so check that every pair and family rule can produce both roles. Specific species pairs win over family rules. The child starts at level one, inherits chosen skills and a plus value; the parents leave.

Add `open: ranch` to a safe town action for reserve swaps, release and breeding. Place it at a location with a background if the breeding scene should use that picture.

Check with `gozen check game` and `gozen sim breed game --format json`; review every unreachable species (GZN0116) and tune with `gozen sim battle` and `gozen sim dive`. `gozen book game` prints recruit and breeding details; `--format json` exports the nested family tree. Expressions read `reserve.size`, `monster.<instance>.family`, `.plus`, `.sex` and `bestiary.<species>.caught`; mutations are the `reserve`, `release` and `breed` commands. The [collection recipe](https://nao1215.github.io/gozen/cookbook/#collect-raise-and-breed-monsters) has a compact YAML example.

## Monster book and births

Set `ui.bestiary: true` and give capturable leaves encounter troops in locations. The screen shows the shortest buildable recipe as up to seven pictures (species, parents, grandparents). Arrows choose a picture; Z makes it the top of a new view, X returns, Tab switches the recipe, Q opens a known species' page. Unseen species show `?` unless `--dev` is on. Its headings and entries can be reworded with `text:` (novel.md).

The birth scene keeps the ranch keeper centered; Normal, Rare and Legendary births use longer pauses and white, gold, or gold plus blue-grey rings, with no rarity text. The result reads `<Name> <sex> was born!`.
