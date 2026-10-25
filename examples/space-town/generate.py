"""Generate the original CC0 neutral sprites used by the Lantern Inn fixture."""
from pathlib import Path
import struct
import zlib

ROOT = Path(__file__).parent / "game" / "assets"
WIDTH = HEIGHT = 64


def png(name, rectangles):
    pixels = bytearray(WIDTH * HEIGHT * 4)
    for x0, y0, x1, y1, rgba in rectangles:
        for y in range(y0, y1):
            for x in range(x0, x1):
                offset = (y * WIDTH + x) * 4
                pixels[offset:offset + 4] = bytes(rgba)
    rows = b"".join(b"\0" + pixels[y * WIDTH * 4:(y + 1) * WIDTH * 4]
                    for y in range(HEIGHT))

    def chunk(kind, data):
        return (struct.pack(">I", len(data)) + kind + data
                + struct.pack(">I", zlib.crc32(kind + data)))

    content = b"\x89PNG\r\n\x1a\n"
    content += chunk(b"IHDR", struct.pack(">IIBBBBB", WIDTH, HEIGHT, 8, 6, 0, 0, 0))
    content += chunk(b"IDAT", zlib.compress(rows)) + chunk(b"IEND", b"")
    (ROOT / name).write_bytes(content)


ROOT.mkdir(parents=True, exist_ok=True)
png("innkeeper.png", [
    (23, 5, 41, 22, (224, 184, 140, 255)),
    (21, 3, 43, 9, (61, 42, 30, 255)),
    (18, 23, 46, 47, (28, 190, 78, 255)),
    (11, 24, 18, 44, (224, 184, 140, 255)),
    (46, 24, 53, 44, (224, 184, 140, 255)),
    (21, 47, 29, 60, (50, 57, 77, 255)),
    (35, 47, 43, 60, (50, 57, 77, 255)),
])
png("bed.png", [
    (5, 17, 59, 55, (107, 65, 33, 255)),
    (9, 12, 55, 44, (208, 35, 48, 255)),
    (12, 13, 52, 24, (244, 233, 196, 255)),
    (6, 50, 13, 62, (75, 45, 24, 255)),
    (51, 50, 58, 62, (75, 45, 24, 255)),
])
