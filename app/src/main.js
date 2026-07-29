const { invoke } = window.__TAURI__.core;
const { listen } = window.__TAURI__.event;
const { getCurrentWindow, LogicalSize } = window.__TAURI__.window;

const RING_COLORS = {
  five_hour: "var(--ok)",
  seven_day: "var(--week)",
  seven_day_opus: "var(--week)",
};

const ui = {
  rings: document.getElementById("rings"),
  outerArc: document.getElementById("outer-arc"),
  innerArc: document.getElementById("inner-arc"),
  innerTrack: document.getElementById("inner-track"),
  glyphClock: document.getElementById("glyph-clock"),
  glyphReset: document.getElementById("glyph-reset"),
  clockValue: document.getElementById("clock-value"),
  clockCaption: document.getElementById("clock-caption"),
  legend: document.getElementById("legend"),
  note: document.getElementById("note"),
  error: document.getElementById("error"),
  autostart: document.getElementById("autostart"),
  statuslineLabel: document.getElementById("statusline-label"),
  statuslineAction: document.getElementById("statusline-action"),
  statuslineHint: document.getElementById("statusline-hint"),
  refresh: document.getElementById("refresh"),
  quit: document.getElementById("quit"),
};

/** Последний снапшот и момент его получения — между опросами тикаем сами. */
let state = { snapshot: null, receivedAt: Date.now() };

function color(window) {
  if (window.level === "warn") return "var(--warn)";
  if (window.level === "critical") return "var(--critical)";
  return RING_COLORS[window.id] ?? "var(--ok)";
}

/** Секунды до сброса с поправкой на время, прошедшее с чтения снапшота. */
function secondsLeft(window) {
  if (window.secondsLeft === null) return null;
  const elapsed = Math.floor((Date.now() - state.receivedAt) / 1000);
  const left = window.secondsLeft - elapsed;
  return left > 0 ? left : null;
}

function countdown(seconds) {
  const minutes = Math.floor(seconds / 60);
  return `${Math.floor(minutes / 60)}:${String(minutes % 60).padStart(2, "0")}`;
}

function plural(count, one, few, many) {
  const mod100 = count % 100;
  if (mod100 >= 11 && mod100 <= 14) return many;
  const mod10 = count % 10;
  if (mod10 === 1) return one;
  if (mod10 >= 2 && mod10 <= 4) return few;
  return many;
}

function ago(seconds) {
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) {
    return `${minutes} ${plural(minutes, "минуту", "минуты", "минут")} назад`;
  }

  const hours = Math.round(minutes / 60);
  if (hours < 24) {
    return `${hours} ${plural(hours, "час", "часа", "часов")} назад`;
  }

  const days = Math.round(hours / 24);
  return `${days} ${plural(days, "день", "дня", "дней")} назад`;
}

function drawArc(element, window) {
  const radius = Number(element.getAttribute("r"));
  const circumference = 2 * Math.PI * radius;
  const progress = window ? Math.min(window.usedPercentage / 100, 1) : 0;

  element.style.strokeDasharray = String(circumference);
  element.style.strokeDashoffset = String(circumference * (1 - progress));
  element.style.stroke = window ? color(window) : "transparent";
}

function render() {
  const data = state.snapshot;
  if (!data) return;

  const five = data.windows.find((w) => w.id === "five_hour");
  const week = data.windows.find((w) => w.id === "seven_day");

  ui.error.hidden = !data.error;
  ui.error.textContent = data.error ?? "";
  ui.rings.hidden = !five;
  ui.legend.hidden = !five;

  if (five) {
    drawArc(ui.outerArc, five);
    drawArc(ui.innerArc, week);
    ui.innerTrack.style.display = week ? "" : "none";

    const left = secondsLeft(five);
    ui.glyphClock.hidden = left === null;
    ui.glyphReset.hidden = left !== null;
    ui.clockValue.textContent = left === null ? "—" : countdown(left);
    ui.clockValue.classList.toggle("dim", left === null);
    ui.clockCaption.textContent = left === null ? "окно сброшено" : "до сброса";

    ui.legend.replaceChildren(
      ...data.windows
        .filter((w) => w.id !== "seven_day_opus")
        .map((w) => {
          const item = document.createElement("li");

          const dot = document.createElement("span");
          dot.className = "dot";
          dot.style.background = color(w);

          const title = document.createElement("span");
          title.textContent = w.title;

          const value = document.createElement("span");
          value.className = "value";
          value.textContent = `${Math.round(w.usedPercentage)}%`;

          item.append(dot, title, value);
          return item;
        }),
    );
  }

  const expired = five?.expired ?? false;
  const stale = data.stale && data.ageSeconds !== null;

  ui.note.hidden = !stale && !expired;
  if (stale && expired) {
    ui.note.textContent = `данные ${ago(data.ageSeconds)} · окно сброшено, отправьте запрос в Claude Code`;
  } else if (stale) {
    ui.note.textContent = `данные ${ago(data.ageSeconds)}`;
  } else if (expired) {
    ui.note.textContent = "окно сброшено, отправьте запрос в Claude Code";
  }

  fitWindow();
}

/** Высота попапа зависит от того, показываем данные или подсказку. */
function fitWindow() {
  const height = Math.ceil(document.querySelector(".popup").getBoundingClientRect().height);
  getCurrentWindow().setSize(new LogicalSize(260, height));
}

function apply(snapshot) {
  state = { snapshot, receivedAt: Date.now() };
  render();
}

async function refreshStatusline() {
  const status = await invoke("statusline_status");

  if (!status.available) {
    ui.statuslineLabel.textContent = "Строка статуса";
    ui.statuslineAction.textContent = "недоступна";
    ui.statuslineAction.disabled = true;
    ui.statuslineAction.className = "link done";
    ui.statuslineHint.hidden = false;
    ui.statuslineHint.className = "hint warning";
    ui.statuslineHint.textContent = "Writer не собран — запустите ./run.sh ещё раз";
    return;
  }

  ui.statuslineAction.disabled = false;

  if (status.installed) {
    ui.statuslineLabel.textContent = "Строка статуса";
    ui.statuslineAction.textContent = "установлена";
    ui.statuslineAction.className = "link done";
    ui.statuslineHint.hidden = true;
    return;
  }

  ui.statuslineLabel.textContent = "Строка статуса";
  ui.statuslineAction.textContent = "установить";
  ui.statuslineAction.className = "link";
  ui.statuslineHint.hidden = false;
  ui.statuslineHint.className = status.conflict ? "hint warning" : "hint";
  ui.statuslineHint.textContent = status.conflict
    ? `Сейчас задано: ${status.conflict}. Установка заменит её, прежние настройки — в settings.json.bak`
    : "Пропишет команду в ~/.claude/settings.json — нужна новая сессия Claude Code";
}

ui.statuslineAction.addEventListener("click", async () => {
  if (ui.statuslineAction.disabled) return;

  try {
    await invoke("install_statusline");
    await refreshStatusline();
    ui.statuslineHint.hidden = false;
    ui.statuslineHint.className = "hint";
    ui.statuslineHint.textContent = "Готово — откройте новую сессию Claude Code";
  } catch (error) {
    ui.statuslineHint.hidden = false;
    ui.statuslineHint.className = "hint warning";
    ui.statuslineHint.textContent = String(error);
  }
  fitWindow();
});

ui.autostart.addEventListener("change", async () => {
  try {
    await invoke("set_autostart", { enabled: ui.autostart.checked });
  } catch (error) {
    ui.autostart.checked = !ui.autostart.checked;
    ui.statuslineHint.hidden = false;
    ui.statuslineHint.className = "hint warning";
    ui.statuslineHint.textContent = String(error);
  }
});

ui.refresh.addEventListener("click", async () => apply(await invoke("snapshot")));
ui.quit.addEventListener("click", () => invoke("quit"));

document.addEventListener("keydown", (event) => {
  if (event.key === "Escape") invoke("hide_popup");
});

listen("snapshot", (event) => apply(event.payload));

// Обратный отсчёт идёт секундами, снапшот перечитывается раз в 15 секунд.
setInterval(render, 1000);

(async () => {
  apply(await invoke("snapshot"));
  ui.autostart.checked = await invoke("autostart_enabled");
  await refreshStatusline();
  fitWindow();
})();
