# ADR 0063: Gozen games inside phone apps

Status: Accepted. Date: 2026-11-08. Implements issue #159.

## Purpose

Authors can offer another standalone Gozen game through an in-game phone. A child uses ordinary YAML and assets, runs in the host process and returns to the phone when closed. Neither a VM nor native plugins are part of this contract.

## Runtime ownership

One Ebitengine loop owns the window and physical devices. It runs one host and at most one child; children cannot launch further children. While a child runs, host gameplay is suspended. The host retains an exit control outside the child's viewport. Portrait and landscape views letterbox the child's logical screen and transform pointer coordinates. Clear held input on entry and exit. Window controls belong to the outer host.

Presentation owns per-game drawing, fonts, language, audio and state lifetimes. Shared device resources may remain process-owned, but closing a child must not reset or close its parent's resources. Game-thread activation of an owned presentation context is allowed; concurrent drawing and updating of different contexts is not. Asynchronous work must capture its instance resources rather than consulting an active context after suspension. Preserve the parent in memory. Saves are scoped by host identity, package identity and release; do not load another release's saves without an explicit compatibility contract. Child results do not automatically grant host rewards.

## Definitions and layers

`internal/layer2/gamedata` owns the host's phone app declarations and their validation. A phone game selects a declared app ID; a phone store lists authored declarations. Each declaration selects a package ID and version constraint, a bundled package path or optional installation, and an optional visibility condition. Existing phone home events remain the entry points. Author declarations cannot add arbitrary executable commands.

`internal/layer2/gamepackage` owns catalog and release definitions, deterministic version resolution, bounded archive validation and explicit installation operations. It is a layer 2 package within the existing private2 boundary. It does not depend on presentation, commands or Ebitengine. Its filesystem loader has no network side effects. Cancellable network and installation calls are explicit; presentation schedules them outside Update/Draw and applies completed results on the game thread. Startup supplies platform storage. Export tooling includes declared bundles through the existing runtime file-selection policy. No new CLI subcommand or architecture exception is required. The maintainer-only `cmd/internal/gamecatalog` executable validates and signs registry snapshots (`go run ./cmd/internal/gamecatalog check ...` and `sign ...`). It belongs to the existing commands component because it consumes game definitions and package validation, like export tooling; the development-analysis component is not granted broader dependencies.

## Registry and trust

Registry source lives under docs/registry in this repository. Each approved release records a namespaced ID, title, author, description, icon, licenses, immutable version, compatible engine versions, orientation, archive URL, SHA-256 and sizes. A custom square static PNG icon is required. The archive includes the icon; validation checks its type and dimensions before presentation uses it. Registration is a review, not automatic publication.

The host pins an authenticated catalog snapshot for reproducible distribution. Network catalog refresh requires either an expected digest or signature verified against a trusted key; a download cannot supply its own trust anchor. Only approved releases in that catalog, allowed by the host and compatible with the installed engine, can be installed. A newer engine requirement never downloads or executes a replacement engine.

Each package is a ZIP with game.yaml at its root and selected YAML, assets and notices. Reject traversal, links, ambiguous names, executable content and unbounded expansion. Verify the archive digest before extraction. Keep the verified ZIP as an immutable archive and expose a bounded read-only filesystem; do not extract game files onto the operating-system filesystem. Stage and validate before publishing an installation. Platform adapters supply bounded fetching and atomic archive storage. A failed or cancelled operation leaves existing installations intact. Installed handles identify an exact release and digest; runtime does not resolve version ranges or download content during loading.

Bundled means present in the host distribution. Optional means a player explicitly downloads it through the phone store. Authors independently choose a fixed version or bounded update range. Updates add immutable installations, never replace an active game, never silently alter saves, and retain previous releases. Catalog changes do not add unsolicited apps to a host. Browser storage support must be verified separately from desktop storage.

## Browser archive storage

Browser installs store verified ZIP bytes in IndexedDB, keyed by the host namespace and archive digest. Small selections and save files retain their existing localStorage format. A 64 MiB archive limit cannot be backed by localStorage: base64 expands the bytes, and even a much smaller valid package can exceed that storage quota. Binary archive storage fixes that mismatch without raising download or expansion limits.

Commit the archive transaction before publishing the selected release. A failed or cancelled write leaves the earlier selection and saves intact. Missing, unavailable or corrupt storage is an installation error, not evidence that a package was installed. Previously cached base64 archives remain readable; there is no save migration command. Explicitly supplied stores retain their existing adapter behavior. Browser quotas and eviction still depend on the origin and browser; the package limit is an upper bound, not a promise of available space.

## Publication and checks

A scheduled workflow detects registered upstream releases and opens metadata update PRs. Validation checks package bytes and compatibility; maintainer review approves publication. Merging registry changes publishes a catalog independently of engine releases. Announcement text must describe only shipped behavior and invite authors to submit games with their icons and distribution rights.

Acceptance requires real portrait and landscape play, closing and reopening a child, parent resource preservation, isolated saves, malformed and interrupted installation recovery, explicit updates, offline installed play, and export checks. Use neutral public fixtures and keep private integration evidence outside tracked files. Document actual platform limits. Add dango checks for ownership and package boundaries, without tests that merely check that this ADR exists.
