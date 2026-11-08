"""Check phone navigation, authored replies and isolated child-game outcomes."""

import hashlib
import json
from pathlib import Path
import sys

run = Path(sys.argv[-1])
records = [json.loads(line) for line in (run / "trace.jsonl").read_text().splitlines()]
if "--games" in sys.argv:
    for app, gold in [("counter", 4), ("trail", 7)]:
        identity = "office-workboard\0fixtures/" + app + "\0" + "1.0.0"
        prefix = "phone-save-" + hashlib.sha256(identity.encode()).hexdigest() + "-"
        state = json.loads((run / "saves" / (prefix + "save1.json")).read_text())["state"]
        if state["gold"] != gold or state["location"] != "room":
            sys.exit(f"child {app} did not reach its saved outcome: {state}")
    state = next(row["state"] for row in reversed(records) if row["t"] == "snapshot")
    if state["gold"] != 0 or state["location"] != "office":
        sys.exit(f"child play changed the host: {state}")
    sys.exit(0)

apps = []
old_history = sent_message = unlocked_post = False
sent = False
for row in records:
    if row.get("t") in ("state", "snapshot"):
        state = row.get("summary", {}) if row["t"] == "state" else row["state"]
        sent = state.get("flags", {}).get("archive_sent", False)
    if row.get("t") != "screen":
        continue
    app = row.get("phone", "")
    if app and (not apps or apps[-1] != app):
        apps.append(app)
    text = " ".join(item["text"] for item in row.get("texts", []) or [])
    old_history |= app == "chat" and "Monday" in text and not sent
    sent_message |= app == "chat" and "I will bring the checklist." in text and sent
    unlocked_post |= app == "feed" and "Checklist confirmed." in text and sent
expected = ["home", "contacts", "chat", "contacts", "home", "chat", "home", "chat", "home", "feed"]
if apps != expected:
    sys.exit(f"phone navigation differs: {apps}; expected {expected}")
if not all((old_history, sent_message, unlocked_post)):
    sys.exit(f"missing phone story: old history={old_history}, sent message={sent_message}, unlocked post={unlocked_post}")
