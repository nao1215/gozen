# Capture, trial and export

## Capture images and GIFs

`gozen capture GAME --config captures.yaml --out DIR` renders each job with fresh state and memory-only saves, so nothing touches real saves. It needs a display (on unattended Linux, `xvfb-run -a`). Output is the images, `shots.json` and `index.html`; the new directory must not exist. A job that fails stays in the report with a reason and no image, other jobs continue, and the exit status is 1 for a partial batch. Use `--only ID` (repeatable), `--format json|jsonl` for a machine-readable report, `--timeout` per job (default 4m), and `--overlay DIR` for approved asset replacements (`--raw` skips approval checks and cannot be combined with `--overlay`).

A job picks a view with `start` (a location), `event` or `scene` (with `line`, `picture`, `answers`), and may set `party`, `level`, `floor`, `flags`, `calm`, `setup` (commands that finish without input) and `wait` (simulation ticks). A `gif` job takes `fps` (1 to 30), `duration` (up to 10 s), `width` (up to 1280) and a `script` of input commands. The [capture section](https://nao1215.github.io/gozen/commands/#capture) has a full config example and all limits.

## Trial (demo) builds

`gozen trial game --out demo` prunes definitions and assets using the range in `trial.yaml`. The analysis is conservative: branches it cannot prove unreachable may remain, and kept files can hold later content. It reports unresolved conditions and rejects output you forbade explicitly. Use `--list FILE`, or `--dry-run`, and inspect the result before shipping.

## Desktop and browser export

`gozen export GAME --out OUT --runtime RUNTIME --target OS-ARCH` needs a matching release runtime. Keep the output directory new and outside both inputs. If the game has an asset policy, first use `release` and export its approved output. Run the packaged player after moving the directory, with isolated saves; check save and reload on the real target, because cross-compiling does not verify the runtime.

For a browser use `--target web` with the matching js-wasm runtime bundle. Game files stay outside WebAssembly and `game/files.json` lists them; the player fetches the whole manifest before play, so do not promise lazy loading. Serve over HTTP(S), also under a subdirectory, and check choices and save/reload in a real browser; saves stay in the origin's `localStorage`.
