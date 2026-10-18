"""Capture the actual sample loop; encode a small slideshow of its PNG frames."""

import json
from pathlib import Path
import shutil
import subprocess
import sys

work = Path(sys.argv[1])
subprocess.run(
    ["build/capture/wayfinder", "--dev", "--mute", "--seed", "7",
     "--window", "1", "--script", "scripts/capture-example.txt",
     "--shots", str(work), "--save-dir", str(work / "saves"),
     "--trace", str(work / "trace.jsonl")],
    check=True, timeout=90,
)
# Check real progress as well as successful image encoding. Scripts may otherwise
# finish normally after a misplaced key or a wait limit.
for slot in (1, 2):
    save = json.loads((work / f"saves/save{slot}.json").read_text())
    state = save["state"]
    if state["location"] != "ridge" or not state["flags"].get("complete") or state["gold"] != 7:
        raise RuntimeError(f"capture did not complete, save and restore the journey (slot {slot})")
trace = [json.loads(line) for line in (work / "trace.jsonl").read_text().splitlines()]
terminal = next(line for line in reversed(trace) if line["t"] == "state" and line.get("terminal"))
if terminal["location"] != "chamber" or terminal["pos"] != [2, 1]:
    raise RuntimeError("capture did not move inside the observation room")
frames = ["00-title", "01-camp", "02-talk", "03-battle", "04-marker",
          "05-ridge", "06-save", "07-loaded", "08-room"]
for index, name in enumerate(frames):
    shutil.copyfile(work / f"{name}.png", work / f"frame-{index:02d}.png")
subprocess.run(
    ["ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-framerate", "1",
     "-reinit_filter", "0", "-i", str(work / "frame-%02d.png"), "-filter_complex",
     "format=rgb24,scale=480:270:flags=lanczos,split[a][b];[a]palettegen[p];[b][p]paletteuse",
     "-loop", "0", "docs/images/wayfinder.gif"], check=True, timeout=30,
)
probe = subprocess.run(
    ["ffprobe", "-v", "error", "-show_entries", "stream=width,height,nb_frames,duration",
     "-of", "json", "docs/images/wayfinder.gif"],
    check=True, stdout=subprocess.PIPE, text=True, timeout=15,
)
stream = json.loads(probe.stdout)["streams"][0]
if (stream["width"], stream["height"], int(stream["nb_frames"]), float(stream["duration"])) != (480, 270, len(frames), float(len(frames))):
    raise RuntimeError("GIF dimensions, frame count or duration do not match the captured sequence")
shutil.copyfile(work / "01-camp.png", "docs/images/wayfinder.png")
print("Captured 9 screens, completed, saved and restored the journey; wrote PNG and GIF to docs/images.")
