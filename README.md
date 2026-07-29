# claude_monitor

Лимиты Claude в строке статуса Claude Code — видно во время работы, без `/usage`.

![Строка статуса в Claude Code](statusline.webp)

## Установка

macOS и Linux:

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

Отключить проверку: `CLAUDESTATUS_NO_AUTO_UPDATE=1`.

## Удаление

```bash
claudestatus uninstall
```

Убирает строку статуса из настроек, кэш, панель на Divoom и сам бинарь.

## Команды

```
claudestatus            строка статуса: JSON сессии на stdin, строка на stdout
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

## Автодополнение

```bash
claudestatus completion zsh >> ~/.zshrc    # или bash >> ~/.bashrc
```

## Divoom Times Gate

Те же лимиты — на экране [Divoom Times Gate](https://divoom.com/products/time-gate).

![Панель лимитов на Divoom Times Gate](divoom.webp)

```bash
claudestatus divoom on
```

Найдёт устройство в сети и включит панель. Дальше она обновляется сама, пока
идёт сессия. Выключить: `claudestatus divoom off`.

Занимает пятый экран, сменить можно на любой:

```bash
claudestatus divoom screen 3
```
Устройство рисует панель через загрузку кадра, поэтому при каждом обновлении
экран моргает индикатором загрузки. Подробности протокола — в [divoom/README.md](divoom/README.md).

## Безопасность

Лимиты приходят от самого Claude Code: он подаёт statusline-команде JSON сессии
на stdin, оттуда и берутся цифры. Ни запросов к API Anthropic, ни токенов, ни
обращений к Keychain — ваши ключи и переписка утилите недоступны. Цифры
обновляются только пока идёт сессия.

Наружу уходят два запроса, оба — не про вас:

- к GitHub — узнать версию последнего релиза и скачать бинарь;
- к каталогу устройств Divoom — спросить адрес вашего Times Gate в локальной
  сети. Каталог отвечает по общему публичному IP и отдаёт только адрес и модель.

Ни почты, ни паролей, ни токенов. Дальше мост работает без интернета: панель
едет к Times Gate по локальному адресу, и отдаётся ему единственная картинка —
та, что вы видите на экране. Забрать её может только само устройство.

