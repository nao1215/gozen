# ADR 0065: Offline web exports

Status: Accepted and implemented (2026-11-19). Home: `cmd/internal/distribute` (the page `index.html`, the worker `sw.js` and its version).

## Context

A web export can already be added to a phone's home screen: the manifest names the game, opens it fullscreen and turned the way its screen is, `web.icon` is its icon, and the game writes the autosave when the page is hidden (ADR 0051, the home screen section of the commands page). It still needs the network every time it opens. A game added to the home screen and opened on a train, or on a phone that has lost its signal, shows the browser's error page instead of the game, although the browser has just downloaded every file of it.

The player downloads the whole game at its start, except the recorded stream voices (`"later": true` in `files.json`), which it fetches when the game plays them (ADR 0051). The files an offline page needs are therefore the ones the page has already read once.

## Decision

- Every web export writes `sw.js` beside the page, and the page registers it. There is no YAML setting: an export a person can open from the home screen should open without a network, and a browser without service workers, or a host that is neither HTTPS nor this computer, plays the page as it did before.
- The worker sends every request of its scope to the network first and keeps each whole reply (status 200 from the same host) in its cache. Only a request the network cannot answer is answered from the cache. An online page therefore always plays the export the host serves now, and an author who exports again sees the new game on the next load. A cache-first worker would show the earlier export until every tab of the page had closed, which makes a re-export look like a failed one.
- The cache is named by the worker's scope and its version, the SHA-256 of the names and bytes of every other file of the export. Two exports of the same files have the same worker. Any change to the game is a new worker whose cache never mixes the files of two exports. Several games under one host keep caches of their own, because the scope is part of the name.
- As it installs, the worker keeps the page, `wasm_exec.js`, the player, the manifest, the icon (when there is one) and `game/files.json`. The rest is kept as the page reads it. Once every file the page counts has come (`goZenProgress("files")` reaches its total), the page tells the worker that controls it. The worker adds the files of `files.json` its cache still lacks (the ones read before it took the page over on a first visit), then removes this game's caches of earlier versions. It keeps them while a newer worker is installing, since that worker's cache is not its to remove, and the page tells the newer worker again when it takes the page over (`controllerchange`). A page that goes offline in the middle of an update therefore still opens, as the earlier export.
- The recorded stream voices are kept only once the game has played them. Added when the files are complete, they would download every stream's voices (hundreds of megabytes in two languages) on a page that plays one stream.
- The worker answers a navigation without a network from the cached page, whatever query it has (`?fps`), and by the scope's address.

## Consequences

The page plays offline as it plays online, including its saves (local storage, ADR 0021). The voices of a stream not yet heard stay unavailable offline. The browser keeps a second copy of the game while the worker exists, in the origin's storage quota. A browser that evicts it under storage pressure leaves an ordinary online page.

`TestWebServiceWorker` checks that the page registers the worker and names the files it keeps, and that the version follows the files. The browser E2E (`e2e/atago/web.atago.yaml`) opens a page once, opens it again with a host that answers nothing, then exports a changed game and checks that the earlier cache is removed and the new game opens offline. It does not run on a physical phone.
