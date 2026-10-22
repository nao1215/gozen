"""Require the feature event in a scripted battle capture."""

import json
import pathlib
import shutil
import sys

feature = sys.argv[1]
run = pathlib.Path(sys.argv[2])
lines = [json.loads(line) for line in (run / "trace.jsonl").read_text().splitlines()]
steps = [line for line in lines if line.get("t") == "battle_step"]
setup = [line for line in lines if line.get("t") == "battle_setup"]
expected_setup = {
    "transform": [("enemy", 0, 680)],
    "merge": [("enemy", 0, 48), ("enemy", 1, 48)],
    "hp": [("enemy", 0, 35), ("party", 0, 15)],
    "motion": [("enemy", 0, 125)],
}
for team, index, hp in expected_setup[feature]:
    if not any(line.get("team") == team and line.get("index") == index and line.get("hp") == hp for line in setup):
        sys.exit(f"capture did not set {team} {index} to {hp} HP")
if feature in ("transform", "merge"):
    kind = {"transform": 12, "merge": 13}[feature]
    result_id = {"transform": "tollkeeper_unsealed", "merge": "king_slime"}[feature]
    if not any(line.get("kind") == kind and isinstance(line.get("target"), dict) and
               line["target"].get("id") == result_id for line in steps):
        sys.exit(f"capture did not show {feature}")
elif feature == "motion":
    if len(list(run.glob("full_*.png"))) < 4:
        sys.exit("capture has too few full-HP Succubus frames")
    if not any(line.get("target", {}).get("id") == "succubus" and line.get("amount", 0) > 0 and 0 < line.get("hp", 0) < 139 for line in steps if isinstance(line.get("target"), dict)):
        sys.exit("capture did not hit the living low-HP Succubus")
elif feature == "hp":
    if not any(line.get("target", {}).get("id") == "drip_slime" and line.get("amount", 0) > 0 for line in steps if isinstance(line.get("target"), dict)):
        sys.exit("capture did not hurt a Drip Slime")
if not list(run.glob("*b*.png")):
    sys.exit("capture wrote no battle frames")

shots = []
for line in lines:
    if line.get("t") == "shot":
        source = run / pathlib.Path(line["file"]).name
        if source.is_file():
            shots.append((line["tick"], source))

if feature == "hp":
    damaged = next((line for line in steps if line.get("kind") == 2 and
                    isinstance(line.get("target"), dict) and
                    line["target"].get("id") == "drip_slime" and
                    0 < line.get("hp", 0) < 24), None)
    if damaged is None:
        sys.exit("capture has no living Drip Slime below half HP")
    fallen = next((line["tick"] for line in steps if line.get("kind") == 7 and
                   isinstance(line.get("target"), dict) and
                   line["target"].get("id") == "drip_slime" and
                   line["tick"] > damaged["tick"]), float("inf"))
    candidates = [(tick, source) for tick, source in shots if damaged["tick"] + 30 <= tick < fallen]
    if not candidates:
        sys.exit("capture has no frame of the living low HP Drip Slime")
    shot_tick, source = candidates[0]
    maren_hp = next((line["hp"] for line in reversed(steps) if line["tick"] <= shot_tick and
                     line.get("kind") in (2, 3) and isinstance(line.get("target"), dict) and
                     line["target"].get("id") == "maren"), 15)
    if not 0 < maren_hp < 18:
        sys.exit("capture has no living low-HP Maren at the selected frame")
    shutil.copyfile(source, run / "s-hurt.png")
else:
    if feature in ("transform", "merge"):
        event = next(line for line in steps if line.get("kind") == kind and
                     isinstance(line.get("target"), dict) and line["target"].get("id") == result_id)
        before, after = 75, 135
    else:
        event = next(line for line in steps if isinstance(line.get("target"), dict) and
                     line["target"].get("id") == "succubus" and line.get("amount", 0) > 0 and 0 < line.get("hp", 0) < 139)
        before, after = 130, 130
    chosen = [source for tick, source in shots if event["tick"] - before <= tick <= event["tick"] + after]
    if len(chosen) < 4:
        sys.exit(f"capture has too few {feature} frames")
    for i, source in enumerate(chosen):
        shutil.copyfile(source, run / f"feature_{i:03}.png")
