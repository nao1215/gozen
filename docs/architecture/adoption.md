# Run and distribute a game

A game is a directory containing `game.yaml`, definitions and assets. It needs no Go module or startup source.

```sh
gozen check game
gozen run game
gozen sim routes game --format json
gozen book game --format json
```

Use `gozen run game --dev --save-dir test-saves` for a separate test play. Capture scripts require `--shots`; promotional capture uses `gozen capture game --config captures.yaml --out promo`.

Desktop and browser exports use a matching prebuilt runtime:

```sh
gozen export game --target web --runtime web-runtime --out site
```

The runtime bundle is built by maintainers, not game authors. Export paths must be new and outside the input. Asset approval policies still apply. `release` prepares approved assets and `trial` selects trial content; neither substitutes for the playable export.

The former root, `schema`, `play`, `gfx`, `puppet`, `chat` and `react` packages are no longer public APIs. First-party session and rendering tests use their owning internal implementations. Arbitrary custom Go loops and callbacks have no automatic YAML replacement. YAML definitions, game identity and save formats remain compatibility boundaries. See [ADR 0041](../decisions/0041-yaml-authoring-and-distribution.md).
