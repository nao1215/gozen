# ADR 0038: Relationship chart in the character book

Status: Accepted

The character book (ADR 0031) also shows a relationship chart. Tab switches between the profile page and the chart. The chart uses the same profile order, unlock record and face crop as the page. A locked character is shown as `?`; edges touching it stay hidden. Development mode reveals all profiles and edges.

`profiles.<id>.affiliation` optionally groups characters. `relations[].kind` selects a line colour from `like`, `love`, `rival`, `enemy`, `family`, `friend`, or `other`; omission means `other`. These are presentation labels. In particular, the engine does not infer `love` or change game rules from it. A relation is directed: an edge without a reverse relation has an arrow. Each direction shows its own `calls` text.

The chart places each affiliation in stable rows with separated face slots. Selection emphasizes incident edges; arrows choose the next face in a direction, Z opens its page, and a click opens the clicked face. The camera follows the selection and the mouse wheel changes scale. `gozen check` rejects unknown kinds and targets. No save format or public API changes are required.
