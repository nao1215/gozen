"""Play Wayfinder from a key script, check that the journey really happened, and encode the GIF.

The game writes the frames from its own Draw loop (script verb `shot`), so the
recording is the real game, not a mock-up. Frames named fNNN.png are the
animation; practice_bNNN.png are the battle frames the game writes while it
plays the practice fight; s-*.png are the still screenshots.
"""

import json
from pathlib import Path
import subprocess
import sys

work = Path(sys.argv[1])
subprocess.run(
    ["build/capture/wayfinder", "--dev", "--mute", "--seed", "7",
     "--window", "1", "--script", "scripts/capture-example.txt",
     "--shots", str(work), "--save-dir", str(work / "saves"),
     "--trace", str(work / "trace.jsonl")],
    check=True, timeout=240,
)

# Check real progress as well as successful encoding. A script can finish
# normally after a misplaced key or a wait limit.
save = json.loads((work / "saves/save1.json").read_text())["state"]
if save["location"] != "ridge" or not save["flags"].get("complete") or save["gold"] != 7 or save["party"] != ["walker", "sela"]:
    raise RuntimeError("capture did not complete and save the journey with Sela (slot 1)")
trace = [json.loads(line) for line in (work / "trace.jsonl").read_text().splitlines()]
states = [line for line in trace if line["t"] == "state"]
visited = {line["location"] for line in states}
if not {"camp", "ridge", "chamber"} <= visited:
    raise RuntimeError(f"capture did not visit every place: {sorted(visited)}")
shown = {name for line in trace if line["t"] == "view" for name in line["pictures"]}
missing = {"title.jpg", "camp.jpg", "ridge.jpg", "town.jpg", "grotto.jpg", "wall.jpg", "courier.png", "innkeeper.png",
           "pebble.png", "walker/normal.png", "floor.jpg", "door.jpg", "gate.jpg", "chest.png", "chest_open.png", "wheel.png",
           "lantern.png", "bram/normal.png", "sela/normal.png"} - shown
if missing:
    raise RuntimeError(f"capture never showed: {sorted(missing)}")
terminal = next(line for line in reversed(states) if line.get("terminal"))
if terminal["location"] != "chamber" or terminal["pos"] != [13, 9]:
    raise RuntimeError("capture did not walk to the lantern in the observation halls")
said = "\n".join(line.get("text", "") for line in trace)
for words in ("Inside the chest", "The sentries roll aside", "You turn the wheel", "From this window"):
    if words not in said:
        raise RuntimeError(f"capture did not show {words!r} in the observation halls")
if terminal["gold"] != 18:
    raise RuntimeError("capture did not collect the chest and win the sentry fight (18G)")

animation = sorted(
    (p for p in work.glob("*.png") if (p.name[0] == "f" and p.name[1:4].isdigit()) or p.name.startswith(("practice_b", "sentry_b"))),
    key=lambda p: p.stat().st_mtime_ns,
)
if len(animation) < 40:
    raise RuntimeError(f"only {len(animation)} frames were captured")
# The party's fight in the halls: Bram's skill shows a cut-in. The trace gives the tick of
# the cut-in and of every frame, so the stills and the kept frames follow the game.
shot_tick = {Path(line["file"]).name: line["tick"] for line in trace if line["t"] == "shot"}
cutins = [line["tick"] for line in trace if line.get("cutin")]
if not cutins:
    raise RuntimeError("the halls' fight showed no cut-in")
cut = cutins[0]
sentry = [p for p in animation if p.name.startswith("sentry_b")]
before = [p for p in sentry if shot_tick[p.name] < cut]
during = [p for p in sentry if cut + 15 <= shot_tick[p.name] <= cut + 50]
if not before or not during:
    raise RuntimeError("no frames of the party or of the cut-in")
stills = {"party": before[-1], "cutin": min(during, key=lambda p: abs(shot_tick[p.name] - cut - 30))}
# Every fourth frame of the fight and the frames of the open cut-in keep the GIF small.
animation = [p for p in animation if not p.name.startswith("sentry_b")
             or sentry.index(p) % 4 == 0 or cut + 15 <= shot_tick[p.name] <= cut + 50]
for name, frame in stills.items():
    subprocess.run(
        ["ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-i", str(frame),
         "-q:v", "3", f"docs/images/wayfinder-{name}.jpg"],
        check=True, timeout=30,
    )
battle = [p for p in animation if p.name.startswith("practice_b")]
if not battle or not any(p.name.startswith("sentry_b") for p in animation):
    raise RuntimeError("no battle frames were captured")
subprocess.run(
    ["ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-i", str(battle[len(battle) // 2]),
     "-q:v", "3", "docs/images/wayfinder-battle.jpg"],
    check=True, timeout=30,
)
# The game writes some frames with an alpha channel. A mixed sequence breaks ffmpeg's
# single palette (every GIF frame alike, or false colors), so every frame becomes RGB first.
for index, frame in enumerate(animation):
    subprocess.run(
        ["ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-i", str(frame), "-pix_fmt", "rgb24",
         str(work / f"frame-{index:03d}.png")],
        check=True, timeout=30,
    )

width, height, fps = 640, 360, 5
Path("docs/images").mkdir(parents=True, exist_ok=True)
subprocess.run(
    ["ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-framerate", str(fps),
     "-i", str(work / "frame-%03d.png"), "-fps_mode", "cfr", "-filter_complex",
     f"format=rgb24,scale={width}:{height}:flags=lanczos,split[a][b];"
     "[a]palettegen=max_colors=112:stats_mode=diff[p];"
     "[b][p]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle",
     "-loop", "0", "docs/images/wayfinder.gif"],
    check=True, timeout=120,
)
probe = subprocess.run(
    ["ffprobe", "-v", "error", "-show_entries", "stream=width,height,duration", "-of", "json", "docs/images/wayfinder.gif"],
    check=True, stdout=subprocess.PIPE, text=True, timeout=15,
)
stream = json.loads(probe.stdout)["streams"][0]
if (stream["width"], stream["height"]) != (width, height) or abs(float(stream["duration"]) - len(animation) / fps) > 0.5:
    raise RuntimeError("GIF dimensions or duration do not match the captured sequence")

# A GIF whose frames all decode alike still has the right size and duration.
md5 = subprocess.run(
    ["ffmpeg", "-hide_banner", "-loglevel", "error", "-i", "docs/images/wayfinder.gif", "-f", "framemd5", "-"],
    check=True, stdout=subprocess.PIPE, text=True, timeout=60,
)
distinct = {line.rsplit(",", 1)[-1].strip() for line in md5.stdout.splitlines() if line and not line.startswith("#")}
if len(distinct) < len(animation) // 3:
    raise RuntimeError(f"GIF frames are too alike ({len(distinct)} distinct of {len(animation)})")

# Still screenshots for the README, as JPEG.
for name in ("title", "talk", "village", "dungeon"):
    subprocess.run(
        ["ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-i", str(work / f"s-{name}.png"),
         "-q:v", "3", f"docs/images/wayfinder-{name}.jpg"],
        check=True, timeout=30,
    )
print(f"Captured {len(animation)} frames, completed, saved and restored the journey; wrote the GIF and screenshots to docs/images.")
