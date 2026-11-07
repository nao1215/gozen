# ADR 0058: Layout records in traces

Status: Accepted and implemented (2026-11-07).

## Authors and the problem

An author who reviews screenshots by eye misses the same faults again and again:
a portrait whose hair is cut by the top of a call's view, a figure that ends in
the air, a line of text that is hard to read on a translucent panel over a light
background. These are visible only once the screen is drawn: the picture's
transparent margin, the frame that clips it, the background behind a glass panel.

## Decision

1. While `--trace` is written, play gathers what each frame draws: every standing
   picture drawn through `drawStanding` or a call view's torso, and every text drawn
   through the play text helpers and the notices. When the gathered layout has
   stayed the same for ten ticks (over two frames at least) it writes one `layout` line.
2. A figure records the picture, the rectangle of its canvas, the rectangle of its
   opaque part (the content box the picture cache already holds), the clip it is
   drawn into, and `source_cut`: the edges its opaque part touches in the file
   itself, so a picture drawn cut is told apart from a picture cut by where it stands.
3. A text records its rectangle, size and color. The screen is read back once per
   written line; the color behind a text is the median of the pixels of its box that
   are not near its own color, and the line carries the WCAG 2 contrast ratio of the
   text, blended by its alpha, on it. A text whose color does not show is left out.
4. gozen records and does not judge. `check` gains no rule and `trace --check` no
   fault for these. Whether a cut head is a close-up, a figure leaving the screen or
   a mistake, and which contrast a game needs, belong to the game's own checks.

## Consequences

- Without `--trace` the cost is a nil check where a figure or a text is drawn.
- With `--trace` the extra cost is one read of the screen per written layout line.
- Moving portraits (puppets) and text drawn directly through gfx (titles, some
  engine screens) are not listed. A text covered by a surface of nearly its own
  color may still be listed with that surface as its back.
- `sim script` draws nothing and writes no layout lines.
