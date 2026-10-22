"""Reject a recruit capture that stopped before the post-battle choice."""

import json
import pathlib
import sys


run = pathlib.Path(sys.argv[1])
events = [json.loads(line) for line in (run / "trace.jsonl").read_text().splitlines()]
won = any(event.get("t") == "battle_end" and event.get("result") == "won" for event in events)
choices = [event for event in events if event.get("t") == "say" and event.get("choices")]
recruit = bool(choices) and choices[-1].get("text") == "Let Drip Slime join?"
if not won or not recruit or choices[-1].get("choices") != ["Yes", "No"]:
    sys.exit("capture did not stop at the Drip Slime recruitment choice")
if not (run / "s-rise.png").is_file():
    sys.exit("capture did not write s-rise.png")
