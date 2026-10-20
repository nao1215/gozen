"""Create an original RGB gradient for the mosaic CLI examples (standard library only)."""
from pathlib import Path
import struct
import zlib
import sys


def chunk(kind, data):
    return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", zlib.crc32(kind + data))


def main():
    width = height = 24
    rows = b"".join(
        b"\0" + bytes(channel for x in range(width) for channel in (x * 8, y * 8, (x + y) * 4))
        for y in range(height)
    )
    png = b"\x89PNG\r\n\x1a\n"
    png += chunk(b"IHDR", struct.pack(">IIBBBBB", width, height, 8, 2, 0, 0, 0))
    png += chunk(b"IDAT", zlib.compress(rows)) + chunk(b"IEND", b"")
    Path(__file__).with_name("pattern.png").write_bytes(png)
    if "--webp" in sys.argv:
        from PIL import Image
        Image.open(Path(__file__).with_name("pattern.png")).save(
            Path(__file__).with_name("pattern.webp"), format="WEBP", lossless=True
        )


if __name__ == "__main__":
    main()
