"""Check that the Wayfinder capture run really played the journey.

`make docs-images` runs the sample with scripts/capture-example.txt (the run
`journey` of docs/images/manifest.yaml) and then this script with the run
directory. A script can finish normally after a misplaced key or a wait
limit, so the save and the trace are checked as well as the exit status.
Python's standard library only.
"""

import json
from pathlib import Path
import sys

work = Path(sys.argv[1])
save = json.loads((work / "saves/save1.json").read_text())["state"]
if save["location"] != "ridge" or not save["flags"].get("complete") or save["gold"] != 7 or save["party"] != ["walker", "sela"]:
    raise SystemExit("capture did not complete and save the journey with Sela (slot 1)")
trace = [json.loads(line) for line in (work / "trace.jsonl").read_text().splitlines()]
states = [line for line in trace if line["t"] == "state"]
visited = {line["location"] for line in states}
if not {"camp", "ridge", "chamber"} <= visited:
    raise SystemExit(f"capture did not visit every place: {sorted(visited)}")
shown = {name for line in trace if line["t"] == "view" for name in line["pictures"]}
missing = {"title.jpg", "camp.jpg", "ridge.jpg", "town.jpg", "grotto.jpg", "wall.jpg", "courier.png", "innkeeper.png",
           "pebble.png", "walker/normal.png", "floor.jpg", "door.jpg", "gate.jpg", "chest.png", "chest_open.png", "wheel.png",
           "lantern.png", "bram/normal.png", "sela/normal.png"} - shown
if missing:
    raise SystemExit(f"capture never showed: {sorted(missing)}")
terminal = next(line for line in reversed(states) if line.get("terminal"))
if terminal["location"] != "chamber" or terminal["pos"] != [13, 9]:
    raise SystemExit("capture did not walk to the lantern in the observation halls")
said = "\n".join(line.get("text", "") for line in trace)
for words in ("Inside the chest", "The sentries roll aside", "You turn the wheel", "From this window"):
    if words not in said:
        raise SystemExit(f"capture did not show {words!r} in the observation halls")
if terminal["gold"] != 18:
    raise SystemExit("capture did not collect the chest and win the sentry fight (18G)")
if not any(line.get("cutin") for line in trace):
    raise SystemExit("the halls' fight showed no cut-in")
print("the journey was played, saved and restored")
