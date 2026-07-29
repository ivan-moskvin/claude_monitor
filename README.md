# claude_monitor

Лимиты Claude в строке статуса Claude Code — видно во время работы, без `/usage`.

## Установка

```bash
curl -fsSL https://raw.githubusercontent.com/ivan-moskvin/claude_monitor/main/install.sh | sh
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/ivan-moskvin/claude_monitor/main/install.ps1 | iex
```

Скрипт качает готовый бинарь последнего релиза, сверяет контрольную сумму, кладёт
его в `~/.local/bin` (Windows — в `%LOCALAPPDATA%\claudestatus` и добавляет каталог
в PATH) и прописывает строку статуса в `~/.claude/settings.json` — прежние настройки
сохраняются рядом, в `settings.json.bak`. Другой каталог — `CLAUDESTATUS_BIN_DIR`.

## Обновление

```bash
claudestatus update
```

Новая версия подсвечивается в конце строки:

![Строка статуса, когда вышла новая версия](docs/update-badge.png)

Отключить проверку: `CLAUDESTATUS_NO_AUTO_UPDATE=1`.

## Удаление

```bash
claudestatus uninstall
```

Убирает строку статуса из настроек, кэш, панель на Divoom и сам бинарь.

## Команды

```
claudestatus            строка статуса: JSON сессии на stdin, строка на stdout
claudestatus install    прописать себя в ~/.claude/settings.json
claudestatus check      проверить, вышла ли новая версия
claudestatus update     скачать последнюю версию и заменить себя ею
claudestatus uninstall  убрать строку статуса, кэш и сам бинарь
claudestatus divoom     показывать лимиты на экране Divoom Times Gate
claudestatus version    показать версию
```

## Строка статуса

Модель, текущий уровень `/effort`, пятичасовое окно, время до его сброса и недельное
окно. Круг рядом с моделью заполняется от `low` (`○`) до `max` (`●`). Цвет полос —
по расходу: зелёный до 60%, оранжевый до 85%, дальше красный.

![Состояния строки статуса](docs/statusline-demo.png)

## Divoom Times Gate

Те же лимиты — на экране [Divoom Times Gate](https://divoom.com/products/time-gate).

```bash
claudestatus divoom --login
```

Спросит почту и пароль от аккаунта Divoom, найдёт устройство в сети и запомнит
токен в `~/.claude/divoom.json`. Дальше панель обновляется сама, пока идёт сессия.

Занимает пятый экран — сменить на другой можно в `lcd_index` того же файла.
Устройство рисует панель через загрузку кадра, поэтому при каждом обновлении
экран моргает индикатором загрузки. Подробности протокола — в [divoom/README.md](divoom/README.md).

## Как это работает

Лимиты приходят от самого Claude Code: он подаёт statusline-команде JSON сессии
на stdin, а `rate_limits` оттуда и печатаются. Никаких запросов к API Anthropic,
токенов и обращений к Keychain — исходники открыты, чтобы это можно было проверить.
В сеть утилита ходит только к GitHub: узнать версию последнего релиза и скачать
из него бинарь.

Цифры обновляются только пока идёт сессия: строку статуса рисует сам Claude Code.
