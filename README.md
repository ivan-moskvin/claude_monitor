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

Ничего доустанавливать не нужно: ни Go, ни компилятора. Строка статуса появится
в следующей сессии Claude Code.

## Обновление

```bash
claudestatus update
```

Утилита качает бинарь нового релиза и заменяет себя им; перезапускать Claude Code
не нужно. Проверяет она и сама — при первом за час вызове строки статуса, в фоне.
Вышла новая версия — в конце строки загорается её номер:

![Строка статуса, когда вышла новая версия](docs/update-badge.png)

Отключить проверку: `CLAUDESTATUS_NO_AUTO_UPDATE=1`.

## Удаление

```bash
claudestatus uninstall
```

Убирает строку статуса из настроек, кэш проверок и сам бинарь.

## Команды

```
claudestatus            строка статуса: JSON сессии на stdin, строка на stdout
claudestatus install    прописать себя в ~/.claude/settings.json
claudestatus check      проверить, вышла ли новая версия
claudestatus update     скачать последнюю версию и заменить себя ею
claudestatus uninstall  убрать строку статуса, кэш и сам бинарь
claudestatus version    показать версию
```

## Строка статуса

Модель, текущий уровень `/effort`, пятичасовое окно, время до его сброса и недельное
окно. Круг рядом с моделью заполняется от `low` (`○`) до `max` (`●`). Цвет полос —
по расходу: зелёный до 60%, оранжевый до 85%, дальше красный.

![Состояния строки статуса](docs/statusline-demo.png)

## Как это работает

Лимиты приходят от самого Claude Code: он подаёт statusline-команде JSON сессии
на stdin, а `rate_limits` оттуда и печатаются. Никаких запросов к API Anthropic,
токенов и обращений к Keychain — исходники открыты, чтобы это можно было проверить.
В сеть утилита ходит только к GitHub: узнать версию последнего релиза и скачать
из него бинарь.

Цифры обновляются только пока идёт сессия: строку статуса рисует сам Claude Code.
