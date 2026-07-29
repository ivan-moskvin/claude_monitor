#!/usr/bin/env python3
"""Рендер демо строки статуса: docs/statusline-demo.html и .png.

Состояния прогоняются через настоящий writer, а его ANSI-вывод переводится
в HTML — картинка не расходится с кодом, потому что рисует её тот же бинарь.

    python3 docs/render-demo.py
"""

import html
import json
import pathlib
import re
import shutil
import subprocess
import sys
import tempfile
import time

ROOT = pathlib.Path(__file__).resolve().parent.parent
OUT_HTML = ROOT / "docs" / "statusline-demo.html"
OUT_PNG = ROOT / "docs" / "statusline-demo.png"
CHROME = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"

SGR = re.compile(r"\x1b\[([0-9;]*)m")


def xterm256(index: int) -> str:
    """Цвет 256-палитры в hex — ровно тот же, что покажет терминал."""
    if index < 16:
        base = [
            "#000000", "#cd0000", "#00cd00", "#cdcd00", "#0000ee", "#cd00cd",
            "#00cdcd", "#e5e5e5", "#7f7f7f", "#ff0000", "#00ff00", "#ffff00",
            "#5c5cff", "#ff00ff", "#00ffff", "#ffffff",
        ]
        return base[index]

    if index < 232:
        index -= 16
        levels = [0, 95, 135, 175, 215, 255]
        r, g, b = levels[index // 36], levels[index // 6 % 6], levels[index % 6]
        return f"#{r:02x}{g:02x}{b:02x}"

    value = 8 + (index - 232) * 10
    return f"#{value:02x}{value:02x}{value:02x}"


def ansi_to_html(line: str) -> str:
    """Мини-парсер SGR: хватает того, что использует writer."""
    state = {"fg": None, "bg": None, "bold": False, "dim": False, "inverse": False}
    chunks, pos = [], 0

    def emit(text: str) -> None:
        if not text:
            return

        fg, bg = state["fg"], state["bg"]
        if state["inverse"]:
            fg, bg = bg or "#1e1e1e", fg or "#d0d0d0"

        style = []
        if fg:
            style.append(f"color:{fg}")
        if bg:
            style.append(f"background:{bg}")
        if state["bold"]:
            style.append("font-weight:700")
        if state["dim"]:
            style.append("opacity:.55")

        body = html.escape(text).replace(" ", "&nbsp;")
        chunks.append(f'<span style="{";".join(style)}">{body}</span>' if style else body)

    for match in SGR.finditer(line):
        emit(line[pos:match.start()])
        pos = match.end()

        codes = [int(c or 0) for c in match.group(1).split(";")]
        index = 0
        while index < len(codes):
            code = codes[index]
            if code == 0:
                state.update(fg=None, bg=None, bold=False, dim=False, inverse=False)
            elif code == 1:
                state["bold"] = True
            elif code == 2:
                state["dim"] = True
            elif code == 7:
                state["inverse"] = True
            elif code == 22:
                state["bold"] = state["dim"] = False
            elif code == 27:
                state["inverse"] = False
            elif code in (38, 48) and codes[index + 1 : index + 2] == [5]:
                key = "fg" if code == 38 else "bg"
                state[key] = xterm256(codes[index + 2])
                index += 2
            index += 1

    emit(line[pos:])
    return "".join(chunks)


def build_writer() -> pathlib.Path:
    binary = pathlib.Path(tempfile.mkdtemp()) / "claude-statusline"
    subprocess.run(
        ["go", "build", "-o", str(binary), "."],
        cwd=ROOT / "statusline", check=True,
    )
    return binary


def render(binary: pathlib.Path, payload: dict) -> str:
    result = subprocess.run(
        [str(binary)], input=json.dumps(payload), capture_output=True, text=True,
    )
    return result.stdout.rstrip("\n")


def session(model="Opus 5", effort=None, five=None, week=None, left=None) -> dict:
    payload = {"model": {"display_name": model}, "rate_limits": {}}

    if effort:
        payload["effort"] = {"level": effort}
    if five is not None:
        window = {"used_percentage": five}
        if left is not None:
            window["resets_at"] = time.time() + left
        payload["rate_limits"]["five_hour"] = window
    if week is not None:
        payload["rate_limits"]["seven_day"] = {"used_percentage": week}

    return payload


def main() -> None:
    binary = build_writer()
    sections = []

    thresholds = [
        ("расход в норме", session(effort="high", five=4, week=11, left=5 * 3600 - 120)),
        ("", session(effort="high", five=23, week=38, left=4 * 3600)),
        ("", session(effort="high", five=47, week=52, left=3 * 3600 + 10 * 60)),
        ("порог 60% — оранжевый", session(effort="high", five=60, week=61, left=2 * 3600 + 20 * 60)),
        ("", session(effort="high", five=74, week=70, left=105 * 60)),
        ("порог 85% — красный", session(effort="high", five=85, week=86, left=40 * 60)),
        ("", session(effort="high", five=96, week=91, left=14 * 60)),
        ("лимит выбран", session(effort="high", five=100, week=99, left=3 * 60)),
    ]
    sections.append(("Расход и обратный отсчёт", thresholds))

    efforts = [
        (f"/effort {level}", session(effort=level, five=42, week=57, left=2 * 3600 + 30 * 60))
        for level in ("low", "medium", "high", "xhigh", "max")
    ]
    efforts.append(("модель без поддержки effort", session(five=42, week=57, left=2 * 3600 + 30 * 60)))
    sections.append(("Уровень reasoning effort — круг рядом с моделью", efforts))

    edges = [
        ("окно уже сброшено — счётчика нет", session(effort="high", five=64, week=71)),
        ("только 5-часовое окно", session(effort="high", five=64, left=90 * 60)),
        ("лимитов в сессии нет", session(effort="high")),
    ]
    sections.append(("Крайние случаи", edges))

    blocks = []
    for title, rows in sections:
        items = []
        for note, payload in rows:
            line = ansi_to_html(render(binary, payload))
            items.append(f'<tr><td class="line">{line}</td><td class="note">{html.escape(note)}</td></tr>')
        blocks.append(f'<h2>{html.escape(title)}</h2><table>{"".join(items)}</table>')

    OUT_HTML.write_text(
        "<!doctype html><meta charset='utf-8'><title>Строка статуса — состояния</title>"
        "<style>"
        "body{background:#1e1e1e;color:#d0d0d0;font:15px/1.9 'SF Mono',Menlo,monospace;padding:32px 36px;margin:0}"
        "h2{font-size:13px;font-weight:600;color:#8a8a8a;letter-spacing:.08em;text-transform:uppercase;"
        "margin:28px 0 10px}h2:first-child{margin-top:0}"
        "table{border-collapse:collapse}td{padding:3px 0;vertical-align:middle;white-space:nowrap}"
        ".note{color:#6f6f6f;font-size:13px;padding-left:28px}"
        "</style>" + "".join(blocks) + "\n"
    )
    print(OUT_HTML)

    if pathlib.Path(CHROME).exists():
        shot = pathlib.Path(tempfile.mkdtemp())
        subprocess.run(
            [CHROME, "--headless", "--disable-gpu", "--hide-scrollbars",
             "--window-size=1180,830", f"--screenshot={OUT_PNG}", OUT_HTML.as_uri()],
            capture_output=True, check=True, cwd=shot,
        )
        shutil.rmtree(shot, ignore_errors=True)
        print(OUT_PNG)
    else:
        print("Chrome не найден — PNG не собран", file=sys.stderr)


main()
