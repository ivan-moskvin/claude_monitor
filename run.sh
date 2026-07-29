#!/usr/bin/env bash
# Собирает виджет из исходников и перезапускает его.
# Приложение работает прямо из репозитория: никакой установки в систему нет.
set -euo pipefail

cd "$(dirname "$0")"
ROOT="$PWD"
TARGET="$ROOT/app/src-tauri/target/release"

# Rust на macOS обычно ставят keg-only rustup или rustup-init — оба каталога
# в PATH неинтерактивной оболочки не попадают.
export PATH="/opt/homebrew/opt/rustup/bin:$HOME/.cargo/bin:$PATH"

missing=()
have() { command -v "$1" >/dev/null 2>&1; }
have go || missing+=("go     — brew install go")
have node || missing+=("node   — brew install node")
have cargo || missing+=("rust   — brew install rustup && rustup default stable")

if [ ${#missing[@]} -gt 0 ]; then
    echo "Не хватает инструментов:" >&2
    printf '  %s\n' "${missing[@]}" >&2
    exit 1
fi

echo "==> Writer"
# Кладём рядом с бинарём: вне бандла Tauri ищет ресурсы в каталоге исполняемого файла.
mkdir -p "$TARGET/resources"
rm -f "$TARGET/resources/claude-statusline"
(cd statusline && go build -trimpath -ldflags="-s -w" -o "$TARGET/resources/claude-statusline" .)

echo "==> Виджет"
[ -d app/node_modules ] || (cd app && npm ci)
(cd app && npx tauri build --no-bundle)

BINARY="$TARGET/claude-usage-bar"
[ -x "$BINARY" ] || { echo "Бинарь не собрался: $BINARY" >&2; exit 1; }

echo "==> Запуск"
pkill -x claude-usage-bar 2>/dev/null || true
sleep 0.3
nohup "$BINARY" >/dev/null 2>&1 &
disown

echo
echo "Виджет в строке меню. Автозапуск и строку статуса включите в попапе."
echo "Остановить: pkill -x claude-usage-bar"
