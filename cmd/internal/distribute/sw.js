"use strict";
// The service worker of a web export (ADR 0065): a page played once opens again without a
// network, from the files it downloaded then.
//
// Every request is sent to the network first, so a page online always plays the export the
// host serves now, and the reply is kept; only a request the network cannot answer is
// answered from what was kept. A reply is kept in the cache of this export's version, the
// SHA-256 of the files beside the page, so the files of two exports are never mixed. The
// caches of earlier versions stay until this one holds every file of game/files.json (the
// page says when the game's files have come, then the missing ones are added): a page
// that goes offline in the middle of an update still opens, as the earlier export.
const version = "VERSION";
const shell = SHELL;
const scope = self.registration.scope;
// Several games can be served by one host, each under its own directory: the caches are
// named by the scope, and only this game's earlier caches are removed.
const prefix = "gozen " + scope + " ";
const current = prefix + version;

self.addEventListener("install", (event) => {
  event.waitUntil(caches.open(current).then((cache) => cache.addAll(shell)).then(() => self.skipWaiting()));
});

self.addEventListener("activate", (event) => {
  event.waitUntil(self.clients.claim());
});

// ours is the names of this game's caches, this version's first.
const ours = async () => {
  const names = (await caches.keys()).filter((name) => name.startsWith(prefix) && name !== current);
  return [current, ...names];
};

const kept = async (request) => {
  for (const name of await ours()) {
    const cache = await caches.open(name);
    // A page is opened with or without a query (?fps) and by its directory or index.html.
    const hit = request.mode === "navigate"
      ? (await cache.match(request, {ignoreSearch: true})) || (await cache.match(scope))
      : await cache.match(request);
    if (hit) return hit;
  }
  return undefined;
};

self.addEventListener("fetch", (event) => {
  const request = event.request;
  if (request.method !== "GET" || !request.url.startsWith(scope)) return;
  event.respondWith((async () => {
    try {
      const reply = await fetch(request);
      // Only a whole reply of this host is kept (not a range, a redirect or an error).
      if (reply.status === 200 && reply.type === "basic") {
        const copy = reply.clone();
        // Kept beside the page reading it, not before: the page counts the bytes as they come.
        event.waitUntil(caches.open(current).then((cache) => cache.put(request, copy)).catch(() => {}));
      }
      return reply;
    } catch (error) {
      const hit = await kept(request);
      if (hit) return hit;
      throw error;
    }
  })());
});

// escapePath escapes a file's name as the player's fetch does (Go's url.URL EscapedPath),
// so the request the page made and the one added here are the same entry of the cache.
const keptBytes = new Set(Array.from("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_.~$&+,/:;=@", (c) => c.charCodeAt(0)));
const escapePath = (name) => Array.from(new TextEncoder().encode(name), (b) =>
  keptBytes.has(b) ? String.fromCharCode(b) : "%" + b.toString(16).toUpperCase().padStart(2, "0")).join("");

// complete adds the files of game/files.json this version's cache does not hold yet, then
// removes this game's earlier caches. A recorded stream voice (later), which the game
// fetches only when it plays it, is kept only once it has been heard: added here, the
// voices of every stream would be downloaded by a page that plays one.
const complete = async () => {
  const cache = await caches.open(current);
  const listed = await cache.match(new URL("game/files.json", scope).href);
  if (!listed) return;
  const files = await listed.json();
  for (const file of files) {
    if (file.later) continue;
    const url = new URL("game/" + escapePath(file.file), scope).href;
    if (!(await cache.match(url))) await cache.add(url);
  }
  // A newer worker being installed has a cache of its own, which is not this one's to
  // remove: the page tells that worker again once it is the page's.
  if (self.registration.installing || self.registration.waiting) return;
  for (const name of await ours()) {
    if (name !== current) await caches.delete(name);
  }
};

self.addEventListener("message", (event) => {
  if (event.data === "gozen-files-complete") event.waitUntil(complete().catch(() => {}));
});
