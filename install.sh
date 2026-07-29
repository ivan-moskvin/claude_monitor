#!/bin/sh
# Ставит claudestatus из последнего релиза. Дальше утилита обслуживает себя сама:
# обновляется claudestatus update, удаляется claudestatus uninstall.
#
#   curl -fsSL https://raw.githubusercontent.com/ivan-moskvin/claude_monitor/main/install.sh | sh
set -eu

REPO="ivan-moskvin/claude_monitor"
BIN_DIR="${CLAUDESTATUS_BIN_DIR:-$HOME/.local/bin}"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in
    darwin|linux) ;;
    *) echo "Неизвестная система: $os — соберите из исходников." >&2; exit 1 ;;
esac

case "$(uname -m)" in
    x86_64|amd64) arch=amd64 ;;
    arm64|aarch64) arch=arm64 ;;
    *) echo "Неизвестная архитектура: $(uname -m) — соберите из исходников." >&2; exit 1 ;;
esac

asset="claudestatus_${os}_${arch}"
base="https://github.com/$REPO/releases/latest/download"

# sha256sum есть в Linux, shasum — в macOS.
if command -v sha256sum >/dev/null 2>&1; then
    sha256() { sha256sum "$1" | cut -d' ' -f1; }
elif command -v shasum >/dev/null 2>&1; then
    sha256() { shasum -a 256 "$1" | cut -d' ' -f1; }
else
    echo "Не нашлось ни sha256sum, ни shasum — нечем проверить загрузку." >&2
    exit 1
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "==> Скачиваю $asset"
curl -fsSL "$base/$asset" -o "$tmp/claudestatus"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"

want="$(grep " \*\{0,1\}$asset\$" "$tmp/checksums.txt" | cut -d' ' -f1)"
if [ -z "$want" ]; then
    echo "В релизе нет контрольной суммы для $asset." >&2
    exit 1
fi
if [ "$want" != "$(sha256 "$tmp/claudestatus")" ]; then
    echo "Контрольная сумма не сошлась — загрузка побилась." >&2
    exit 1
fi

mkdir -p "$BIN_DIR"
chmod +x "$tmp/claudestatus"
mv "$tmp/claudestatus" "$BIN_DIR/claudestatus"
echo "==> Поставлен в $BIN_DIR/claudestatus"

"$BIN_DIR/claudestatus" install
