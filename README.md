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

## Безопасность

Лимиты приходят от самого Claude Code: он подаёт statusline-команде JSON сессии
на stdin, оттуда и берутся цифры. Ни запросов к API Anthropic, ни токенов, ни
обращений к Keychain — ваши ключи и переписка утилите недоступны. Цифры
обновляются только пока идёт сессия.

Наружу уходят два запроса, оба — не про вас:

- к GitHub — узнать версию последнего релиза и скачать бинарь;
- к облаку Divoom при `divoom --login` — почта и пароль от **вашего аккаунта
  Divoom**, чтобы получить токен устройства. Так же делает их приложение.

Пароль нигде не сохраняется — в `~/.claude/divoom.json` (права `600`) ложится
только токен, дающий доступ к устройству в вашей локальной сети. Дальше мост
работает без интернета: панель едет к Times Gate по локальному адресу, и
отдаётся ему единственная картинка — та, что вы видите на экране.
