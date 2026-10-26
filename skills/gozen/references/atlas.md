# Maps (atlas)

A location shows as a map when it has `map:` (schema version 3 or later): one `picture`, `points` (each may lead `to` a location, say a line while `locked`, or run `do`) and `roads` between points. Paint the roads in the picture; the road lines in YAML are what the token walks and are drawn only with `draw_roads: true`. `unknown` sets how unvisited points look: `hidden`, `dim`, `mark` or `named`.

## Hide closed points

`hide: closed` on a point removes the point, marker, name plate and its roads while its `when` is false. Keyboard, pointer, selection-panel and headless choices skip it. Without it, a closed point is a gray pin.

<!-- verify: parse fragment of a map point; the flag is set by a game event -->
```yaml
workshop:
  at: [640, 360]
  label: Workshop
  hide: closed
  when: flag.workshop_built
  do: [{say: Welcome.}]
```

## Selection panel and instant actions

`map.selection_panel` shows the selected point's advice in a fixed rectangle (`rect: [x, y, w, h]` in screen fractions; `portrait` is the default face). Each point's `advice` is an ordered list of `{when, heading, text, portrait}`; the first variant whose `when` holds wins, and text can read state with `{item.potion}`. Clicking selects, Confirm (or the panel's button) activates, Back hides it. `map.activation: instant` makes the map an action menu: every open point with a `do` or `to` is available without roads (the current point too), and nothing walks, rolls road encounters or changes the saved spot. The two options are independent, and neither is saved.

<!-- verify: parse fragment of a map; the pictures are the author's -->
```yaml
map:
  picture: maps/valley.png
  activation: instant
  selection_panel: {portrait: people/guide.png}
  points:
    town:
      at: [120, 240]
      start: true
      do: [{say: "Welcome back."}]
      advice: [{heading: Town guide, text: "Rest before the next expedition."}]
    quarry:
      at: [320, 180]
      to: quarry
      advice:
        - {when: "item.potion < 3", text: "Bring three potions. You have {item.potion}."}
        - {text: "Your supplies are ready for the quarry."}
```

## Camera for a large map

`camera: {zoom: 3, max_zoom: 8}` lets the player pan and zoom (`camera: {}` starts at zoom 2, maximum 8; leave `camera` out for the fitted overview). Labels keep their size, and looking around changes no state. Arrows select the nearest point, Tab cycles, `+`/`-` or the wheel zoom, Home shows the whole map, End returns to the party. The camera is not saved.

## Campaign maps

On a campaign's map, regions use `area` polygons in picture pixels; see campaign.md.

## Check it

`gozen check` reports bad point names, areas outside the picture and missing pictures. Headless tools such as `sim design` see every point as a menu entry, so a map can be tested without a window.
