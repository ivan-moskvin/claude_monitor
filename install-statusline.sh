#!/bin/bash
set -euo pipefail

TARGET="$HOME/.claude/statusline.py"
SETTINGS="$HOME/.claude/settings.json"

mkdir -p "$HOME/.claude"

cat > "$TARGET" <<'SCRIPT'
#!/usr/bin/env python3
import datetime
import json
import pathlib
import sys
import time

SNAPSHOT = pathlib.Path.home() / ".claude" / "usage-snapshot.json"
WIDTH = 10


def bar(percentage: float) -> str:
    filled = int(round(percentage / 100 * WIDTH))
    filled = max(0, min(WIDTH, filled))
    return "█" * filled + "░" * (WIDTH - filled)


def countdown(resets_at) -> str | None:
    if not isinstance(resets_at, (int, float)):
        return None
    seconds = int(resets_at - time.time())
    if seconds <= 0:
        return "сброс"
    minutes, _ = divmod(seconds, 60)
    hours, minutes = divmod(minutes, 60)
    return f"{hours}ч {minutes:02d}м" if hours else f"{minutes}м"


def main() -> None:
    try:
        data = json.load(sys.stdin)
    except Exception:
        print("—")
        return

    limits = data.get("rate_limits") or {}

    if limits:
        payload = {
            "rate_limits": limits,
            "updated_at": datetime.datetime.now(datetime.timezone.utc).isoformat(),
        }
        tmp = SNAPSHOT.with_suffix(".tmp")
        tmp.write_text(json.dumps(payload))
        tmp.replace(SNAPSHOT)

    parts = []

    five = limits.get("five_hour") or {}
    used = five.get("used_percentage")
    if used is not None:
        segment = f"5h {bar(used)} {used:.0f}%"
        left = countdown(five.get("resets_at"))
        if left:
            segment += f" ({left})"
        parts.append(segment)

    week = limits.get("seven_day") or {}
    used = week.get("used_percentage")
    if used is not None:
        parts.append(f"7d {bar(used)} {used:.0f}%")

    model = (data.get("model") or {}).get("display_name")
    if model:
        parts.append(model)

    print(" · ".join(parts) if parts else "—")


main()
SCRIPT

chmod +x "$TARGET"

python3 - "$SETTINGS" <<'PY'
import json
import pathlib
import shutil
import sys

path = pathlib.Path(sys.argv[1])
settings = {}

if path.exists():
    shutil.copy(path, path.with_suffix(".json.bak"))
    try:
        settings = json.loads(path.read_text())
    except Exception:
        print("settings.json не разбирается — оставляю как есть, правь вручную")
        raise SystemExit(1)

existing = settings.get("statusLine")
wanted = {"type": "command", "command": "python3 $HOME/.claude/statusline.py"}

if existing and existing != wanted:
    print(f"ВНИМАНИЕ: statusLine уже занят: {json.dumps(existing, ensure_ascii=False)}")
    print("Заменяю. Прежнее значение — в settings.json.bak")

settings["statusLine"] = wanted
path.write_text(json.dumps(settings, indent=2, ensure_ascii=False) + "\n")
print("statusLine настроен")
PY

echo "Готово. Открой новую сессию claude — строка появится внизу."
