"""Build neutral, reproducible phone packages for CLI and graphics E2E tests.

Child games and their generated PNG icons are original MIT-licensed fixtures.
They use the engine's built-in font and contain no third-party assets."""
import base64
import hashlib
import io
import json
from pathlib import Path
import shutil
import struct
import sys
import zipfile
import zlib

source, destination = map(Path, sys.argv[1:3])
mode = sys.argv[3] if len(sys.argv) > 3 else "valid"
if mode != "packages-only":
    shutil.copytree(source, destination)
    game_config = destination / "game.yaml"
    game_config.write_text(game_config.read_text().replace("start: {location: camp, party: [scout]}", "start: {location: camp, party: [scout], event: phone_home}"))
(destination / "apps").mkdir(exist_ok=True)
if mode != "packages-only":
    (destination / "apps/undeclared.zip").write_bytes(b"not selected for export")



def chunk(kind, data):
    return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", zlib.crc32(kind + data))


def noise_png():
    """A valid reproducible image whose ZIP exceeds localStorage capacity."""
    width, height = 1536, 2048
    pixels = hashlib.shake_256(b"Gozen original storage fixture").digest(width * height * 3)
    rows = b"".join(b"\x00" + pixels[y * width * 3:(y + 1) * width * 3] for y in range(height))
    return b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", width, height, 8, 2, 0, 0, 0)) + chunk(b"IDAT", zlib.compress(rows, level=0)) + chunk(b"IEND", b"")


def png(red, green, blue):
    rows = []
    for y in range(32):
        row = bytearray(b"\x00")
        for x in range(32):
            color = (red, green, blue)
            if red < 100 and any((x-cx)**2 + (y-cy)**2 <= 25 for cx, cy in [(11, 11), (21, 20)]):
                color = (255, 221, 94)
            if red >= 100 and abs(x - (10 if y < 12 else 21 if y < 22 else 14)) < 3:
                color = (240, 240, 215)
            row.extend(color)
        rows.append(bytes(row))
    pixels = b"".join(rows)
    return b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", 32, 32, 8, 2, 0, 0, 0)) + chunk(b"IDAT", zlib.compress(pixels)) + chunk(b"IEND", b"")


releases = []
for index, (name, width, height) in enumerate((("counter", 360, 540), ("trail", 640, 360))):
    icon = png(55 + index * 80, 130, 185 - index * 70)
    game = f"""schema_version: 11
id: phone-child-{name}
title: {name.title()}
screen: {{width: {width}, height: {height}}}
start: {{location: room, gold: 3}}
"""
    world = """locations:
  room:
    name: Practice room
    people:
      - name: Keeper
        talk:
          - set: {gold: gold + 1}
          - say: 'Collected {gold} tokens.'
    actions:
      - label: Count tokens
        do: [{say: 'Tokens: {gold}'}]
"""
    if name == "trail":
        world = """locations:
  room:
    name: Trail marker
    actions:
      - label: Choose a route
        do:
          - prompt: Which trail?
            choice:
              - {label: Riverside, do: [{set: {gold: gold + 2}}, {say: Riverside marker reached.}]}
              - {label: Hill, do: [{set: {gold: gold + 4}}, {say: Hill marker reached.}]}
"""
    files = {"game.yaml": game.encode(), "data/world.yaml": world.encode(),
             "assets/icon.png": icon,
             "LICENSE": (Path(__file__).resolve().parents[4] / "LICENSE").read_bytes()}
    version = "1.0.0"
    if mode in ("browser-large", "browser-large-update") and name == "trail":
        files["assets/storage.png"] = noise_png()
    if mode in ("browser-large-update", "browser-small-update") and name == "trail":
        version = "1.1.0"
        files["game.yaml"] = files["game.yaml"].replace(b"title: Trail", b"title: Trail updated")
    if mode == "traversal" and index == 0:
        files["../outside.txt"] = b"must not be published"
    buffer = io.BytesIO()
    with zipfile.ZipFile(buffer, "w", compression=zipfile.ZIP_DEFLATED) as archive:
        for filename, contents in sorted(files.items()):
            info = zipfile.ZipInfo(filename, date_time=(2020, 1, 1, 0, 0, 0))
            info.create_system = 3  # Keep archive metadata identical on Windows and Unix.
            info.compress_type = zipfile.ZIP_DEFLATED
            info.external_attr = 0o100644 << 16
            archive.writestr(info, contents)
    package = buffer.getvalue()
    (destination / "apps" / (name + ".zip")).write_bytes(package)
    release = dict(id="fixtures/" + name, version=version, title=name.title(), author="Gozen contributors",
                   description="Collect tokens." if index == 0 else "Choose a trail and collect its reward.",
                   icon="assets/icon.png", icon_data=base64.b64encode(icon).decode(), license="MIT",
                   orientation="portrait" if index == 0 else "landscape",
                   url="https://example.com/fixtures/" + name + "-" + version + ".zip",
                   sha256=hashlib.sha256(package).hexdigest(), size=len(package),
                   unpacked_size=sum(map(len, files.values())), engine=">=0.23.0 <0.24.0")
    if mode != "packages-only":
        # This all-zero commit is synthetic and intercepted by browser E2E tests.
        release["browser_url"] = "https://raw.githubusercontent.com/nao1215/gozen/" + "0" * 40 + "/test-fixtures/" + name + ".zip"
    releases.append(release)
# JSON is also valid YAML and makes byte-stable catalog hashing straightforward.
catalog = (json.dumps(dict(schema=1, releases=releases), sort_keys=True, indent=2) + "\n").encode()
(destination / "apps/catalog.yaml").write_bytes(catalog)
if mode == "corrupt":
    archive = destination / "apps/counter.zip"
    damaged = bytearray(archive.read_bytes())
    damaged[len(damaged) // 2] ^= 1
    archive.write_bytes(damaged)
template = Path(__file__).with_name("host.yaml").read_text()
digest = "f" * 64 if mode == "catalog-digest" else hashlib.sha256(catalog).hexdigest()
phone = template.replace("a" * 64, digest)
if mode in ("browser-large-update", "browser-small-update"):
    phone = phone.replace("      version: 1.0.0", "      version: '>=1.0.0 <2.0.0'")
if mode == "packages-only":
    print(phone, end="")
    sys.exit(0)
with (destination / "game.yaml").open("a") as out:
    out.write("\n" + phone)
# A separate file avoids duplicate events mappings in the existing fixture.
(destination / "data/phone.yaml").write_text("""events:
  phone_home:
    do:
      - say: Open the games phone.
      - phone:
          app: home
          title: Games
          rows:
            - {button: Counter, event: phone_counter}
            - {button: Trail, event: phone_trail}
            - {button: Store, event: phone_store}
      - say: Back in the host.
  phone_counter:
    do: [{phone: {app: game, game: counter, hold: portrait}}]
  phone_trail:
    do: [{phone: {app: game, game: trail, hold: landscape}}]
  phone_store:
    do: [{phone: {app: store, title: Game library}}]
""")
print("Prepared", mode, "phone packages")
