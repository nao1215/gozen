"""Check the filmed stair ascent and extract its short, real-time GIF frames."""

import json
import pathlib
import subprocess
import sys

run = pathlib.Path(sys.argv[1])
# Trim the initial wall-facing turn and stop before the return descent.
start, duration, fps = 19.0, 10.5, 12
rows = [json.loads(line) for line in (run / "trace.jsonl").read_text().splitlines()]
levels = {row["level"] for row in rows
          if row.get("t") == "state" and start * 60 <= row["tick"] < (start + duration) * 60}
if levels != {0, 1, 2}:
    sys.exit(f"short capture must visit the ground, second and third floors; got {levels}")
subprocess.run([
    "ffmpeg", "-loglevel", "error", "-y", "-ss", str(start),
    "-i", str(run / "tour.mp4"), "-t", str(duration),
    "-vf", f"fps={fps}", "-threads", "1", str(run / "stairs-%04d.png"),
], check=True)
frames = list(run.glob("stairs-*.png"))
if len(frames) != int(duration * fps):
    sys.exit(f"expected {int(duration * fps)} frames, got {len(frames)}")
