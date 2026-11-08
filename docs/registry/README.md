# Approved phone game catalog

The catalog lists reviewed game releases for the phone store. Host authors choose which package IDs and version ranges to offer. Publishing a catalog does not install an app or change an active game. The catalog includes Counter, an example maintained with Gozen. The office sample uses its own local fixture catalog.

`catalog.json` is the reviewed source. `sources.json` lists repositories from which the scheduled updater may propose new versions. Each source has `id` (`owner/name`), `repository` (`owner/repository`) and `manifest_asset` (`gozen-package.json`). Registering a source and its first release requires maintainer review, including distribution rights for the game and icon. The game registration issue form collects that information.

## Upstream release contract

Publish a stable GitHub release with a ZIP and a `gozen-package.json` asset. The metadata is one release object with these fields:

| Field | Contract |
| --- | --- |
| `id`, `version` | Approved namespaced ID; stable `major.minor.patch`, without a prefix or prerelease suffix |
| `title`, `author`, `description`, `license` | Nonempty display information and license identifier or explanation |
| `orientation` | `portrait` or `landscape` |
| `icon` | Portable path to the custom square static PNG inside the ZIP |
| `icon_data` | Standard base64 of the same PNG, at most 64 KiB decoded and 32–1024 pixels square |
| `url` | HTTPS GitHub release-asset URL in the registered repository and the same release tag as the metadata |
| `browser_url` | Optional direct CORS mirror: `https://raw.githubusercontent.com/OWNER/REPO/FULL_COMMIT/PATH.zip`, with a lowercase 40-hex commit in the same approved repository. It must serve exactly the native ZIP bytes. Required for new browser downloads; ordinary GitHub release URLs redirect and cannot serve this role. |
| `sha256`, `size` | Lowercase SHA-256 and exact ZIP byte count |
| `unpacked_size` | Exact sum of the sizes of all regular files in the ZIP |
| `engine` | Exact stable Gozen version, or a bounded interval such as `>=0.23.0 <0.24.0` |

The ZIP has `game.yaml` at its root, ordinary `data/`, `lang/`, `streams/`, `assets/` and license notices. It contains no executable code, hooks or dependencies that install other games. Memory/browser installations are limited to 64 MiB compressed, 256 MiB expanded and 4096 entries. Desktop archives backed by a file allow 1 GiB compressed, 2 GiB expanded and 32768 entries; verification streams their bytes and keeps the archive open instead of cloning it. Native cache publication requires hard-link support in the cache directory; unsupported filesystems report an installation error and preserve existing data. Central directories are limited to 16 MiB and ZIP64/multi-volume archives are unsupported. Validation rejects links, traversal, Windows-reserved paths, hidden components, case collisions, corrupt entries and executable files. The icon in the catalog must exactly match the icon in the archive.

Browser mirrors must also fit the memory/browser limits. Registry validation downloads both locations and verifies the same digest and icon; a different browser build requires its own release.

The updater reads the latest 100 GitHub releases of each registered repository, ignores drafts and prereleases, and proposes metadata additions. It never executes upstream programs. A changed existing ID/version is an error. Corrections require a new version; do not replace an existing release asset with different bytes.

## Review and publication

The Game catalog workflow validates every archive's size, digest, paths, icon and game definitions before opening an update PR. Review still covers distribution rights, content, compatibility claims and actual gameplay. Validation does not prove execution on every supported platform.

Merging changes to the registry starts validation and publishes `catalog.json` and `catalog.sig` as assets of the dedicated `game-catalog` GitHub release. That release is not marked latest and does not trigger the engine's `v*` release workflow. Publication is independent of engine releases. Engine snapshot builds explicitly select the nearest reachable `v*` tag and exclude catalog/game tags from discovery; follow the [release checklist](../development/release-checklist.md) to select an engine version. The publishing job also creates one Git commit containing both signed assets on the dedicated `catalog-data` branch. Browser hosts use the CORS-compatible raw endpoints `https://raw.githubusercontent.com/nao1215/gozen/catalog-data/catalog.json` and `https://raw.githubusercontent.com/nao1215/gozen/catalog-data/catalog.sig`. The branch is a delivery channel; the pinned public key authenticates its bytes. The updater does not sign or publish its own candidates.

The publishing job requires the repository secret `GAME_CATALOG_SIGNING_KEY`: a standard-base64 Ed25519 seed of exactly 32 bytes. Keep this private key in the secret store. The signer requires its derived public key to match the reviewed `public-key.txt` before publication. Hosts pin the corresponding standard-base64 32-byte public key; never obtain the trusted key from the catalog being downloaded. The signature is standard base64 over the exact catalog bytes. Secret provisioning and the first publication must be completed before signed refresh is advertised as available.

GitHub replaces the two release assets separately. The raw branch advances as one commit, but two requests can still straddle a branch update or CDN refresh. A reader may briefly receive a mismatched pair during publication. Signature verification must fail, preserve its previously authenticated catalog and allow retry. After refresh, the entire authenticated envelope is stored atomically for offline use. Existing releases cannot be removed or modified by an accepted refresh.

The update job uses the repository token to open a PR. Repository settings must permit GitHub Actions to create pull requests. Workflows triggered by the default Actions token may not create another workflow run; the update job therefore performs the same archive validation before opening its PR. A maintainer must close and reopen the bot PR using their own GitHub account (or `gh pr close NUMBER` followed by `gh pr reopen NUMBER` authenticated as that maintainer) to emit a fresh `pull_request` event. Confirm that all required checks actually run and pass before merging; the updater job's archive check does not substitute for PR CI.

## Maintainer checks

These commands are maintainer tools, not new author-facing CLI commands:

```sh
python3 scripts/update-game-catalog.py
go run ./cmd/internal/gamecatalog check docs/registry/catalog.json docs/registry/sources.json
go test -race ./internal/layer2/gamepackage ./cmd/internal/gamecatalog
```

To check immutability against an earlier trusted snapshot, supply its filename as the final argument to `check`. `sign CATALOG SIGNATURE` uses the signing-key environment variable and writes only the detached signature. It never prints the private key.
