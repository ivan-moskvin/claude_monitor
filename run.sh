#!/usr/bin/env bash
# Собирает строку статуса из исходников и прописывает её в Claude Code.
# Бинарь живёт прямо в репозитории: никакой установки в систему нет.
set -euo pipefail

cd "$(dirname "$0")"
ROOT="$PWD"
BINARY="$ROOT/bin/claude-statusline"

ASSUME_YES=0
case "${1:-}" in
    -y|--yes) ASSUME_YES=1 ;;
    "") ;;
    *) echo "Неизвестный аргумент: $1 (есть только --yes)" >&2; exit 2 ;;
esac

export PATH="/opt/homebrew/bin:$PATH"

have() { command -v "$1" >/dev/null 2>&1; }

# Go ставим сами: руками остаётся только Homebrew, если его нет.
ensure_go() {
    have go && return

    echo "Не хватает: go"

    if ! have brew; then
        echo "Сначала нужен Homebrew — дальше скрипт всё поставит сам:" >&2
        echo '  /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/brew/HEAD/install.sh)"' >&2
        exit 1
    fi

    if [ "$ASSUME_YES" != 1 ]; then
        if [ ! -t 0 ]; then
            echo "Нет терминала для вопроса — запустите с --yes." >&2
            exit 1
        fi
        # Пустой ответ — это Enter, а вот EOF (Ctrl-D) читается как отказ.
        if ! read -r -p "Поставить через brew? [Y/n] " answer; then
            echo >&2
            echo "Отменено." >&2
            exit 1
        fi
        case "$answer" in
            ""|y|Y|yes|Yes|да) ;;
            *) echo "Отменено." >&2; exit 1 ;;
        esac
    fi

    echo "==> Устанавливаю go"
    brew install go

    hash -r
    have go || { echo "go не нашёлся и после установки — откройте новый терминал и повторите." >&2; exit 1; }
}

ensure_go

echo "==> Сборка"
mkdir -p "$ROOT/bin"
(cd statusline && go build -trimpath -ldflags="-s -w" -o "$BINARY" .)

echo "==> Установка"
"$BINARY" --install

echo
echo "Готово. Строка статуса появится в следующей сессии Claude Code."
