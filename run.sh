#!/usr/bin/env bash
# Собирает claudestatus из исходников и прописывает строку статуса в Claude Code.
# Бинарь живёт прямо в репозитории, наружу торчит только ссылка в ~/.local/bin.
set -euo pipefail

cd "$(dirname "$0")"
ROOT="$PWD"
BINARY="$ROOT/bin/claudestatus"
LINK_DIR="$HOME/.local/bin"

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

# Версию вшиваем в бинарь: по ней claudestatus check понимает, отстал ли клон.
VERSION="$(git -C "$ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)"

echo "==> Сборка ($VERSION)"
mkdir -p "$ROOT/bin"
(cd claudestatus && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o "$BINARY" .)

# Бинарь до переименования утилиты — иначе в bin/ остаётся мёртвый файл.
rm -f "$ROOT/bin/claude-statusline"

echo "==> Ссылка в $LINK_DIR"
mkdir -p "$LINK_DIR"
ln -sfn "$BINARY" "$LINK_DIR/claudestatus"

echo "==> Установка"
"$BINARY" install

echo
echo "Готово. Строка статуса появится в следующей сессии Claude Code."

case ":$PATH:" in
    *":$LINK_DIR:"*) echo "Команда: claudestatus help" ;;
    *) echo "Чтобы команда claudestatus вызывалась откуда угодно, добавьте в ~/.zshrc:"
       echo "  export PATH=\"$LINK_DIR:\$PATH\"" ;;
esac
