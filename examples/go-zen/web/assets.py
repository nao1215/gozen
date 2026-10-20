#!/usr/bin/env python3
"""assets.py - write the game's files for the browser build of Go Zen.

    python3 web/assets.py SRC DST                       # game -> .../play/go-zen/game
    python3 web/assets.py SRC DST PAGE OUT WASM         # and the page, with the sizes

The page downloads every file of the game before it starts, so the pictures are made
smaller on the way: a PNG or JPEG becomes a lossy WebP of the same size in pixels (the
scenes, maps and puppets place things in the pixels of their pictures, so none is
resized), with its alpha when it has one. A picture that is not 8-bit (the 16-bit depth
map of a painted scene) or that WebP would not make smaller is copied as it is, and so
is everything else (YAML, music, the font).

DST/files.json lists the files for the program (main_js.go): for each one the name the
game reads ("assets/bg/plaza.jpg"), the file the page serves ("assets/bg/plaza.webp")
and its size in bytes, so the page can show how much is left. The game reads a picture
by its content, not by its name, so a WebP under the name of a PNG is read as WebP.
The PC build and the files under SRC do not change.

A second run converts only what changed: a file written from a source that is not newer
than it is kept (files.json remembers whether the source became a WebP), and files whose
source is gone are removed. Remove DST to convert everything again after changing the
settings below.

With PAGE, OUT and WASM it also writes the page PAGE to OUT with @WASM_SIZE@ (the bytes of
the program WASM) and @DOWNLOAD_MB@ (the program and the game's files) filled in.

It needs Pillow built with WebP support (python3-pil on Debian and Ubuntu).
"""

import json
import os
import shutil
import sys
from concurrent.futures import ProcessPoolExecutor

from PIL import Image, features

QUALITY = 80
ALPHA_QUALITY = 90
PICTURES = (".png", ".jpg", ".jpeg")


def convert(job):
    """Write the file the page serves for SRC; returns (rel, served path, converted)."""
    src, dst, rel, before = job
    if before is not None:
        out = os.path.join(dst, before)
        if os.path.exists(out) and os.path.getmtime(out) >= os.path.getmtime(src):
            return rel, before, False
    stem, ext = os.path.splitext(rel)
    os.makedirs(os.path.join(dst, os.path.dirname(rel)), exist_ok=True)
    if ext.lower() in PICTURES:
        with Image.open(src) as im:
            if im.mode in ("RGB", "RGBA", "L", "LA", "P"):
                alpha = im.mode in ("RGBA", "LA") or "transparency" in im.info
                im = im.convert("RGBA" if alpha else "RGB")
                out = stem + ".webp"
                path = os.path.join(dst, out)
                im.save(path, "WEBP", quality=QUALITY, alpha_quality=ALPHA_QUALITY, method=6)
                if os.path.getsize(path) < os.path.getsize(src):
                    return rel, out, True
                os.remove(path)
    # copy2 keeps the source's time, so the copy counts as up to date next time.
    shutil.copy2(src, os.path.join(dst, rel))
    return rel, rel, True


def previous(dst):
    """The served file of each name in the files.json of an earlier run, or {}."""
    try:
        with open(os.path.join(dst, "files.json"), encoding="utf-8") as f:
            return {e["name"]: e["file"] for e in json.load(f)}
    except (OSError, ValueError, KeyError, TypeError):
        return {}


def write_page(page, out, wasm, files_total):
    wasm_size = os.path.getsize(wasm)
    with open(page, encoding="utf-8") as f:
        html = f.read()
    html = html.replace("@WASM_SIZE@", str(wasm_size))
    html = html.replace("@DOWNLOAD_MB@", f"{(wasm_size + files_total) / 1e6:.0f}")
    with open(out, "w", encoding="utf-8") as f:
        f.write(html)


def main():
    if len(sys.argv) not in (3, 6):
        sys.exit("usage: assets.py SRC DST [PAGE OUT WASM]")
    src, dst = sys.argv[1], sys.argv[2]
    if not features.check("webp"):
        sys.exit("assets.py: this Pillow cannot write WebP (install python3-pil, or pillow with libwebp)")
    if os.path.abspath(dst) == os.path.abspath(src):
        sys.exit("assets.py: DST must not be SRC")
    before = previous(dst)
    jobs = []
    for root, _, names in os.walk(src):
        for name in sorted(names):
            rel = os.path.relpath(os.path.join(root, name), src).replace(os.sep, "/")
            jobs.append((os.path.join(root, name), dst, rel, before.get(rel)))
    with ProcessPoolExecutor() as pool:
        served = list(pool.map(convert, jobs, chunksize=4))
    files, seen = [], {}
    for name, out, _ in sorted(served):
        if out in seen:
            sys.exit(f"assets.py: {seen[out]} and {name} would both be served as {out}")
        seen[out] = name
        files.append({"name": name, "file": out, "size": os.path.getsize(os.path.join(dst, out))})
    # What an earlier run wrote for sources that are gone, or that are served otherwise now.
    removed = 0
    for root, _, names in os.walk(dst, topdown=False):
        for name in names:
            rel = os.path.relpath(os.path.join(root, name), dst).replace(os.sep, "/")
            if rel != "files.json" and rel not in seen:
                os.remove(os.path.join(root, name))
                removed += 1
        if root != dst and not os.listdir(root):
            os.rmdir(root)
    with open(os.path.join(dst, "files.json"), "w", encoding="utf-8") as f:
        json.dump(files, f, separators=(",", ":"))
    converted = sum(1 for s in served if s[2])
    size_before = sum(os.path.getsize(j[0]) for j in jobs)
    after = sum(f["size"] for f in files)
    print(f"assets.py: {len(files)} files ({converted} written, {removed} removed), "
          f"{size_before / 1e6:.1f} MB -> {after / 1e6:.1f} MB under {dst}")
    if len(sys.argv) == 6:
        write_page(sys.argv[3], sys.argv[4], sys.argv[5], after)


if __name__ == "__main__":
    main()
