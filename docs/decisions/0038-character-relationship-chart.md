# ADR 0038: Relationship chart in the character book

Status: Accepted

The character book (ADR 0031) also shows a relationship chart. Tab switches between the profile page and the chart. The chart uses the same profile order, unlock record and face crop as the page. A locked character is shown as `?`; edges touching it stay hidden. Development mode reveals all profiles and edges.

`profiles.<id>.affiliation` optionally groups characters. `relations[].kind` selects a line colour from `like`, `love`, `rival`, `enemy`, `family`, `friend`, or `other`; omission means `other`. These are presentation labels. In particular, the engine does not infer `love` or change game rules from it. A relation is directed: an edge without a reverse relation has an arrow. The chart has a colour and arrow legend; `calls` remains on the detail page. A relation to `you` adds a neutral player node.

The chart places profiles in compact, stable rows with separated face slots. Routes around unrelated face slots are chosen deterministically. Arrows choose the next face in a direction; Z or a click focuses an unlocked face. Over 20 frames, the chosen face moves to the centre and its incoming and outgoing neighbours, including You when related, form a circle. Other faces and their lines fade almost completely. Z again opens the chosen page; X from focus or from that page returns to the full chart over 20 frames, while F6 closes the book. The camera follows the selection in the full chart and the mouse wheel changes scale. `gozen check` rejects unknown kinds and targets. No save format or public API changes are required.
