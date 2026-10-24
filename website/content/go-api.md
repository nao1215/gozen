---
title: "Migrating from the Go API"
description: "Run existing YAML games with the CLI and export them with prebuilt runtimes."
---

The public Go API has been retired. Keep your game's YAML, assets and game ID; remove the Go host from the authoring workflow.

<!-- verify: run fixture=cave -->
```sh
gozen check game
gozen sim routes game --format json
gozen book game --format json
```

Run the checked directory with `gozen run game`.

Use [export](/commands/#export) to package a game with a prebuilt desktop or browser runtime. Authors need no compiler. A `go.mod` is not required for a game directory.

The root, `schema`, `play`, `gfx`, `puppet`, `chat` and `react` packages are no longer importable application APIs. Their retained implementation lives in internal packages. Custom Go loops and callbacks do not have an automatic YAML translation. Moving portraits remain available through actor and enemy YAML and `gozen puppet`.

This change does not change YAML schema versions or save formats. Preserve the game ID and save directory when continuing an existing game. See [commands](/commands/), [writing a game](/writing-a-game/) and the [cookbook](/cookbook/).
