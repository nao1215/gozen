// Opens the browser build of Go Zen in headless Chrome and checks that the game draws:
// the page's loading note must be gone and the screen must show a picture (many
// colours, not a black or one-colour screen) within the time limit. It speaks the
// DevTools protocol over Node's own WebSocket, so it needs no packages.
//
//	node scripts/web_play_check.mjs URL [-shots DIR] [-enter N] [-timeout SECONDS] [-max-frame MS]
//	node scripts/web_play_check.mjs -serve DIR PATH ...   (serves DIR itself, opens PATH)
//
// It prints the page's console, its exceptions and failed requests, and fails on any
// of them. With -shots it writes title.png (the first picture drawn) and, with
// -enter N, enter.png after N presses of Enter. It prints how long the page took to
// fetch the game's files and then to draw the first frame (from the page's goZenTimes),
// and the longest the page stood still after that (no animation frame). The page is
// opened with ?fps, so the game shows its frame meter and puts its numbers in
// window.gozenStats each second; the check prints the longest Update and Draw and the
// lowest FPS after the first frame, and with -max-frame MS fails when the longest
// Update or Draw took longer than MS milliseconds. CHROME names the browser (default
// google-chrome); its profile goes under TMPDIR and is removed afterwards.
import { spawn } from "node:child_process";
import { existsSync, mkdtempSync, mkdirSync, readFileSync, rmSync, statSync, writeFileSync } from "node:fs";
import { createServer } from "node:http";
import { tmpdir } from "node:os";
import { extname, join, resolve, sep } from "node:path";
import { inflateSync } from "node:zlib";

const args = process.argv.slice(2);
let url = "";
let serve = "";
let shots = "";
let enters = 0;
let timeout = 120;
let maxFrame = 0;
for (let i = 0; i < args.length; i++) {
  if (args[i] === "-shots") shots = args[++i];
  else if (args[i] === "-enter") enters = Number(args[++i]);
  else if (args[i] === "-timeout") timeout = Number(args[++i]);
  else if (args[i] === "-serve") serve = args[++i];
  else if (args[i] === "-max-frame") maxFrame = Number(args[++i]);
  else url = args[i];
}
if (!url) {
  console.error("usage: web_play_check.mjs [-serve DIR] URL|PATH [-shots DIR] [-enter N] [-timeout SECONDS] [-max-frame MS]");
  process.exit(2);
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// A static server for DIR on a free port of 127.0.0.1, as GitHub Pages serves the site.
const types = {
  ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".json": "application/json",
  ".wasm": "application/wasm", ".png": "image/png", ".jpg": "image/jpeg", ".webp": "image/webp",
  ".svg": "image/svg+xml", ".ogg": "audio/ogg", ".ttf": "font/ttf", ".yaml": "text/yaml",
};
let server;
if (serve) {
  const root = resolve(serve);
  server = createServer((req, res) => {
    let p = resolve(join(root, decodeURIComponent(new URL(req.url, "http://localhost").pathname)));
    if (p !== root && !p.startsWith(root + sep)) p = "";
    if (p && existsSync(p) && statSync(p).isDirectory()) p = join(p, "index.html");
    if (!p || !existsSync(p)) {
      res.writeHead(404).end();
      return;
    }
    res.writeHead(200, { "Content-Type": types[extname(p)] || "application/octet-stream" });
    res.end(readFileSync(p));
  });
  await new Promise((r) => server.listen(0, "127.0.0.1", r));
  url = `http://127.0.0.1:${server.address().port}${url}`;
}
// The frame meter of the game (the browser build shows it with ?fps).
url += url.includes("?") ? "&fps" : "?fps";

const profile = mkdtempSync(join(process.env.TMPDIR || tmpdir(), "gozen-chrome-"));
const chrome = spawn(process.env.CHROME || "google-chrome", [
  "--headless=new", "--remote-debugging-port=0", `--user-data-dir=${profile}`,
  "--no-first-run", "--no-default-browser-check", "--mute-audio",
  "--use-angle=swiftshader", "--enable-unsafe-swiftshader", "--window-size=1280,720",
  "about:blank",
], { stdio: "ignore" });

let failed = false;
let ws;
function finish(code) {
  try { ws?.close(); } catch { /* closing anyway */ }
  chrome.kill();
  server?.close();
  setTimeout(() => {
    rmSync(profile, { recursive: true, force: true });
    process.exit(code);
  }, 500);
}

async function target() {
  for (let i = 0; i < 100; i++) {
    try {
      // Chrome writes the port it took to DevToolsActivePort in its profile.
      const port = readFileSync(join(profile, "DevToolsActivePort"), "utf8").split("\n")[0];
      const list = await (await fetch(`http://127.0.0.1:${port}/json/list`)).json();
      const page = list.find((t) => t.type === "page");
      if (page) return page.webSocketDebuggerUrl;
    } catch { /* not up yet */ }
    await sleep(200);
  }
  throw new Error("Chrome did not start");
}

let seq = 0;
const waiting = new Map();
function send(method, params = {}) {
  const id = ++seq;
  ws.send(JSON.stringify({ id, method, params }));
  return new Promise((ok, fail) => waiting.set(id, { ok, fail }));
}

// The pixels of a PNG that Chrome wrote (8-bit RGB or RGBA, not interlaced).
function decodePNG(buf) {
  let p = 8;
  let w = 0, h = 0, ch = 0;
  const idat = [];
  while (p < buf.length) {
    const len = buf.readUInt32BE(p);
    const type = buf.toString("latin1", p + 4, p + 8);
    const data = buf.subarray(p + 8, p + 8 + len);
    if (type === "IHDR") {
      w = data.readUInt32BE(0);
      h = data.readUInt32BE(4);
      if (data[8] !== 8 || data[12] !== 0) throw new Error("unexpected PNG");
      ch = { 2: 3, 6: 4 }[data[9]];
      if (!ch) throw new Error("unexpected PNG colour type");
    } else if (type === "IDAT") idat.push(data);
    p += 12 + len;
  }
  const raw = inflateSync(Buffer.concat(idat));
  const stride = w * ch;
  const px = Buffer.alloc(h * stride);
  for (let y = 0; y < h; y++) {
    const f = raw[y * (stride + 1)];
    const src = raw.subarray(y * (stride + 1) + 1, (y + 1) * (stride + 1));
    for (let i = 0; i < stride; i++) {
      const a = i >= ch ? px[y * stride + i - ch] : 0;
      const b = y > 0 ? px[(y - 1) * stride + i] : 0;
      const c = i >= ch && y > 0 ? px[(y - 1) * stride + i - ch] : 0;
      let v = src[i];
      if (f === 1) v += a;
      else if (f === 2) v += b;
      else if (f === 3) v += (a + b) >> 1;
      else if (f === 4) {
        const q = a + b - c, pa = Math.abs(q - a), pb = Math.abs(q - b), pc = Math.abs(q - c);
        v += pa <= pb && pa <= pc ? a : pb <= pc ? b : c;
      }
      px[y * stride + i] = v & 255;
    }
  }
  return { w, h, ch, px };
}

// A screenshot of the page and how many distinct colours (5 bits a channel) a grid of
// its pixels has: a black or one-colour screen has one or two, a scene hundreds.
// (The game's WebGL canvas cannot be read back by a script of the page, so the check
// looks at what the browser shows.)
async function look() {
  const r = await send("Page.captureScreenshot", { format: "png" });
  const png = Buffer.from(r.data, "base64");
  const { w, h, ch, px } = decodePNG(png);
  const seen = new Set();
  for (let y = 0; y < h; y += 8) {
    for (let x = 0; x < w; x += 8) {
      const i = (y * w + x) * ch;
      seen.add((px[i] >> 3) << 10 | (px[i + 1] >> 3) << 5 | (px[i + 2] >> 3));
    }
  }
  return { colors: seen.size, w, h, png };
}

// The page's loading note, or "" once it is gone.
async function note() {
  const r = await send("Runtime.evaluate", {
    returnByValue: true,
    expression: `document.getElementById("note")?.textContent ?? ""`,
  });
  return r.result.value;
}

async function shot(name, png) {
  if (!shots) return;
  if (!png) png = (await look()).png;
  mkdirSync(shots, { recursive: true });
  writeFileSync(join(shots, name), png);
  console.log(`wrote ${join(shots, name)}`);
}

async function key(type) {
  await send("Input.dispatchKeyEvent", {
    type, key: "Enter", code: "Enter", windowsVirtualKeyCode: 13, nativeVirtualKeyCode: 13,
    ...(type === "keyDown" ? { text: "\r" } : {}),
  });
}

// The page's times: fetching the files, from them to the first frame, the longest gap
// between animation frames after the first frame, and the frame meter's numbers after
// it. It returns false when a frame took longer than -max-frame.
async function timings() {
  const r = await send("Runtime.evaluate", {
    returnByValue: true,
    expression: `({ t: window.goZenTimes || {}, gaps: window.goZenGaps || [], frames: window.goZenFrames || [] })`,
  });
  const { t, gaps, frames } = r.result.value;
  const s = (ms) => (ms / 1000).toFixed(2);
  if (t.fetched !== undefined && t.ready !== undefined) {
    console.log(`files fetched at ${s(t.fetched)}s; the first frame ${s(t.ready - t.fetched)}s after them` +
      (t.starting !== undefined ? ` (the game started ${s(t.starting - t.fetched)}s after them)` : ""));
  }
  if (t.ready !== undefined) {
    // A gap that began before the first frame counts from it: the page can stand still
    // from just before the first frame is shown to long after.
    let worst = [0, 0];
    for (const [at, gap] of gaps) {
      const from = Math.max(at, t.ready), still = at + gap - from;
      if (still > worst[1]) worst = [from, still];
    }
    console.log(`longest stand-still after the first frame: ${s(worst[1])}s (from ${s(worst[0])}s)`);
  }
  // The meter's first second can hold the first frame, so it counts from a second after.
  const after = frames.filter(([at]) => t.ready !== undefined && at >= t.ready + 1000).map(([, f]) => f);
  if (after.length === 0) {
    console.log("no frame meter numbers after the first frame (window.gozenStats)");
    return maxFrame <= 0;
  }
  const most = (k) => Math.max(...after.map((f) => f[k]));
  const fps = Math.min(...after.map((f) => f.fps));
  const upd = most("updateMaxMs"), draw = most("drawMaxMs");
  console.log(`frame meter over ${after.length}s: longest Update ${upd.toFixed(1)}ms, longest Draw ${draw.toFixed(1)}ms, ` +
    `lowest FPS ${fps.toFixed(1)}, average Draw ${(after.reduce((a, f) => a + f.drawAvgMs, 0) / after.length).toFixed(1)}ms`);
  if (maxFrame > 0 && Math.max(upd, draw) > maxFrame) {
    console.log(`FAIL: a frame took ${Math.max(upd, draw).toFixed(1)}ms, over -max-frame ${maxFrame}ms`);
    return false;
  }
  return true;
}

async function main() {
  ws = new WebSocket(await target());
  await new Promise((r, e) => { ws.onopen = r; ws.onerror = e; });
  ws.onmessage = (m) => {
    const msg = JSON.parse(m.data);
    if (msg.id && waiting.has(msg.id)) {
      const w = waiting.get(msg.id);
      waiting.delete(msg.id);
      if (msg.error) w.fail(new Error(msg.error.message)); else w.ok(msg.result);
      return;
    }
    const p = msg.params;
    if (msg.method === "Runtime.consoleAPICalled") {
      console.log(`console.${p.type}: ${p.args.map((a) => a.value ?? a.description ?? "").join(" ")}`);
    } else if (msg.method === "Runtime.exceptionThrown") {
      failed = true;
      console.log(`exception: ${p.exceptionDetails.exception?.description ?? p.exceptionDetails.text}`);
    } else if (msg.method === "Network.loadingFailed") {
      failed = true;
      console.log(`request failed: ${p.errorText} (${p.type})`);
    } else if (msg.method === "Network.responseReceived" && p.response.status >= 400 &&
      !p.response.url.endsWith("/favicon.ico")) {
      failed = true;
      console.log(`HTTP ${p.response.status}: ${p.response.url}`);
    }
  };
  await send("Runtime.enable");
  await send("Network.enable");
  await send("Page.enable");
  await send("Emulation.setDeviceMetricsOverride", { width: 1280, height: 720, deviceScaleFactor: 1, mobile: false });
  // Every animation frame notes the time since the one before: a long gap is the page
  // standing still (the program holding the browser's one thread).
  await send("Page.addScriptToEvaluateOnNewDocument", {
    source: `window.goZenGaps = [];
      { let last = 0;
        const tick = (t) => { if (last) goZenGaps.push([last, t - last]); last = t; requestAnimationFrame(tick); };
        requestAnimationFrame(tick); }
      window.goZenFrames = [];
      { let last;
        setInterval(() => {
          const s = window.gozenStats;
          if (s && s !== last) { last = s; goZenFrames.push([performance.now(), s]); }
        }, 250); }`,
  });
  const start = Date.now();
  await send("Page.navigate", { url });

  let seen = null;
  let last = null;
  let said = "";
  const secs = () => ((Date.now() - start) / 1000).toFixed(1);
  while (Date.now() - start < timeout * 1000) {
    await sleep(1000);
    const n = await note().catch(() => said);
    if (n !== said && n) console.log(`${secs()}s: ${n}`);
    said = n;
    if (n.startsWith("Could not start")) break;
    last = await look().catch(() => last);
    if (!n && last && last.colors >= 64) { seen = last; break; }
  }
  if (!seen) {
    if (last) await shot("title.png", last.png);
    console.log(`FAIL: nothing drawn after ${secs()}s (${last ? last.colors : 0} colours on the screen)`);
    return finish(1);
  }
  console.log(`drawn after ${secs()}s: ${seen.w}x${seen.h}, ${seen.colors} colours`);
  await sleep(1500);
  await shot("title.png");
  for (let i = 0; i < enters; i++) {
    await key("keyDown");
    await sleep(100);
    await key("keyUp");
    await sleep(1500);
  }
  if (enters) {
    await sleep(1500);
    await shot("enter.png");
  }
  const smooth = await timings();
  if (failed) console.log("FAIL: the page logged an error (above)");
  finish(failed || !smooth ? 1 : 0);
}

main().catch((e) => { console.log(`FAIL: ${e.message}`); finish(1); });
