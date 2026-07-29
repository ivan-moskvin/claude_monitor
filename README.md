# claude_monitor

Лимиты Claude в строке статуса Claude Code — видно во время работы, без `/usage`.

## Установка

```bash
go install github.com/ivan-moskvin/claude_monitor/claudestatus@latest
claudestatus install
```

Нужен `go`: `brew install go`, в Windows — `winget install GoLang.Go`.

Первая команда кладёт бинарь в `~/go/bin`, вторая прописывает его
в `~/.claude/settings.json` — прежние настройки сохраняются рядом,
в `settings.json.bak`. Если `~/go/bin` нет в PATH, `install` подскажет строку
для `~/.zshrc`. Строка статуса появится в следующей сессии Claude Code.

## Обновление

```bash
claudestatus update
```

Утилита узнаёт последнюю версию и переустанавливает себя ею — клон репозитория
для этого не нужен. Проверяет она и сама: при первом за час вызове строки статуса,
в фоне. Вышла новая версия — в строке загорается `↑ v1.2.3`. Отключить проверку:
`CLAUDESTATUS_NO_AUTO_UPDATE=1`.

## Команды

```
claudestatus            строка статуса: JSON сессии на stdin, строка на stdout
claudestatus install    прописать себя в ~/.claude/settings.json
claudestatus check      проверить, вышла ли новая версия
claudestatus update     переустановить себя последней версией
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
Единственный сетевой запрос — номер последней версии у `proxy.golang.org`, того же
прокси, через который утилита ставится.

Цифры обновляются только пока идёт сессия: строку статуса рисует сам Claude Code.
