#!/usr/bin/env python3
"""assets.py - write the game's files for the browser build of Go Zen.

    python3 web/assets.py SRC DST      # make website-go-zen: game -> .../play/go-zen/game

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
    """Write the file the page serves for SRC; returns its path relative to DST."""
    src, dst, rel = job
    stem, ext = os.path.splitext(rel)
    if ext.lower() in PICTURES:
        with Image.open(src) as im:
            if im.mode in ("RGB", "RGBA", "L", "LA", "P"):
                alpha = im.mode in ("RGBA", "LA") or "transparency" in im.info
                im = im.convert("RGBA" if alpha else "RGB")
                out = stem + ".webp"
                path = os.path.join(dst, out)
                im.save(path, "WEBP", quality=QUALITY, alpha_quality=ALPHA_QUALITY, method=6)
                if os.path.getsize(path) < os.path.getsize(src):
                    return rel, out
                os.remove(path)
    shutil.copyfile(src, os.path.join(dst, rel))
    return rel, rel


def main():
    if len(sys.argv) != 3:
        sys.exit("usage: assets.py SRC DST")
    src, dst = sys.argv[1], sys.argv[2]
    if not features.check("webp"):
        sys.exit("assets.py: this Pillow cannot write WebP (install python3-pil, or pillow with libwebp)")
    if os.path.abspath(dst) == os.path.abspath(src):
        sys.exit("assets.py: DST must not be SRC")
    shutil.rmtree(dst, ignore_errors=True)
    jobs = []
    for root, _, names in os.walk(src):
        for name in sorted(names):
            rel = os.path.relpath(os.path.join(root, name), src).replace(os.sep, "/")
            os.makedirs(os.path.join(dst, os.path.dirname(rel)), exist_ok=True)
            jobs.append((os.path.join(root, name), dst, rel))
    with ProcessPoolExecutor() as pool:
        served = list(pool.map(convert, jobs, chunksize=4))
    files, seen = [], {}
    for name, out in sorted(served):
        if out in seen:
            sys.exit(f"assets.py: {seen[out]} and {name} would both be served as {out}")
        seen[out] = name
        files.append({"name": name, "file": out, "size": os.path.getsize(os.path.join(dst, out))})
    with open(os.path.join(dst, "files.json"), "w", encoding="utf-8") as f:
        json.dump(files, f, separators=(",", ":"))
    before = sum(os.path.getsize(j[0]) for j in jobs)
    after = sum(f["size"] for f in files)
    print(f"assets.py: {len(files)} files, {before / 1e6:.1f} MB -> {after / 1e6:.1f} MB under {dst}")


if __name__ == "__main__":
    main()
