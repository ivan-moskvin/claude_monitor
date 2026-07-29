package i18n

// The Russian catalog: English source text on the left, translation on the
// right. Adding a language means copying this file and registering the map in
// catalogs — nothing else in the project knows about languages.
//
// Format verbs and their order are part of the key: a translation has to keep
// them, or the message prints garbage.
var ru = map[string]string{
	// claudestatus: help and arguments
	`claudestatus — Claude limits in the Claude Code status line.

Usage:
  claudestatus            status line: session JSON on stdin, line on stdout
  claudestatus install    register in ~/.claude/settings.json
  claudestatus check      check whether a new version is out
  claudestatus update     download the latest version and replace itself
  claudestatus uninstall  remove the status line, the cache and the binary
  claudestatus divoom     limits panel on a Divoom Times Gate (divoom help)
  claudestatus completion shell: completion script (zsh|bash)
  claudestatus version    print the version
  claudestatus help       this help

Environment:
  CLAUDESTATUS_LANG=ru|en         force the interface language
  CLAUDESTATUS_NO_AUTO_UPDATE=1   do not check for updates in the background
`: `claudestatus — лимиты Claude в строке статуса Claude Code.

Использование:
  claudestatus            строка статуса: JSON сессии на stdin, строка на stdout
  claudestatus install    прописать себя в ~/.claude/settings.json
  claudestatus check      проверить, вышла ли новая версия
  claudestatus update     скачать последнюю версию и заменить себя ею
  claudestatus uninstall  убрать строку статуса, кэш и сам бинарь
  claudestatus divoom     панель лимитов на Divoom Times Gate (divoom help)
  claudestatus completion оболочка: скрипт автодополнения (zsh|bash)
  claudestatus version    показать версию
  claudestatus help       эта справка

Переменные окружения:
  CLAUDESTATUS_LANG=ru|en         задать язык интерфейса
  CLAUDESTATUS_NO_AUTO_UPDATE=1   не проверять обновления в фоне
`,
	"Unknown command: %s\n\n%s": "Неизвестная команда: %s\n\n%s",
	"built from source":         "сборка из исходников",

	// claudestatus: status line
	"reset":     "сброс",
	"%dh %02dm": "%dч %02dм",

	// claudestatus: completion
	`Usage:
  claudestatus completion zsh   >> ~/.zshrc
  claudestatus completion bash  >> ~/.bashrc
`: `Использование:
  claudestatus completion zsh   >> ~/.zshrc
  claudestatus completion bash  >> ~/.bashrc
`,
	`_claudestatus() {
  local -a commands
  commands=(
    'install:register in the Claude Code settings'
    'check:check whether a new version is out'
    'update:update to the latest version'
    'uninstall:remove the status line and the binary'
    'divoom:limits panel on a Divoom Times Gate'
    'completion:shell completion script'
    'version:print the version'
    'help:help'
  )
  local -a divoom_commands
  divoom_commands=(
    'on:find the device and turn the panel on'
    'off:turn the panel off'
    'once:send the panel once'
    'screen:show or take over screen 1-5'
    'preview:save a frame to a file'
    'help:help'
  )

  if (( CURRENT == 2 )); then
    _describe 'command' commands
  elif (( CURRENT == 3 )) && [[ ${words[2]} == divoom ]]; then
    _describe 'command' divoom_commands
  fi
}
compdef _claudestatus claudestatus
`: `_claudestatus() {
  local -a commands
  commands=(
    'install:прописать себя в настройки Claude Code'
    'check:проверить, вышла ли новая версия'
    'update:обновиться до последней версии'
    'uninstall:убрать строку статуса и сам бинарь'
    'divoom:панель лимитов на Divoom Times Gate'
    'completion:скрипт автодополнения для оболочки'
    'version:показать версию'
    'help:справка'
  )
  local -a divoom_commands
  divoom_commands=(
    'on:найти устройство и включить панель'
    'off:выключить панель'
    'once:отправить панель один раз'
    'screen:показать или занять экран 1-5'
    'preview:сохранить кадр в файл'
    'help:справка'
  )

  if (( CURRENT == 2 )); then
    _describe 'команда' commands
  elif (( CURRENT == 3 )) && [[ ${words[2]} == divoom ]]; then
    _describe 'команда' divoom_commands
  fi
}
compdef _claudestatus claudestatus
`,

	// claudestatus: install and uninstall
	"%s does not parse — fix it by hand":                                  "%s не разбирается — поправьте его вручную",
	"could not back up settings.json: %w":                                 "не удалось сделать бэкап settings.json: %w",
	"could not read settings.json: %w":                                    "не удалось прочитать settings.json: %w",
	"Replacing the previous status line: %s\n":                            "Заменяю прежнюю строку статуса: %s\n",
	"The previous settings are in %s.bak\n":                               "Прежние настройки — в %s.bak\n",
	"could not create %s: %w":                                             "не удалось создать %s: %w",
	"Status line registered in %s\n":                                      "Строка статуса прописана в %s\n",
	"\n%s is not in PATH — the claudestatus command will not be found.\n": "\nКаталог %s не в PATH — команда claudestatus не найдётся.\n",
	"Line for ~/.zshrc:  export PATH=\"%s:$PATH\"\n":                      "Строка для ~/.zshrc:  export PATH=\"%s:$PATH\"\n",
	"Removed %s\n":           "Удалён %s\n",
	"Removed the cache %s\n": "Удалён кэш %s\n",
	"The binary is still there — remove it by hand: %s\n":                 "Бинарь остался — удалите вручную: %s\n",
	"Removed the binary %s\n":                                             "Удалён бинарь %s\n",
	"There is no %s — nothing to clean in the settings\n":                 "%s нет — настройки чистить не нужно\n",
	"There is no status line in %s\n":                                     "В %s строки статуса нет\n",
	"%s holds a status line that is not ours — leaving it as is:\n  %s\n": "В %s прописана не наша строка статуса — оставляю как есть:\n  %s\n",
	"Status line removed from %s (the previous settings are in %s.bak)\n": "Строка статуса убрана из %s (прежние настройки — в %s.bak)\n",
	"could not determine our own path: %w":                                "не удалось определить свой путь: %w",

	// claudestatus: version check and update
	"%s installed, %s is out — update with: claudestatus update\n":                       "Установлено %s, вышло %s — обновиться: claudestatus update\n",
	"Not a release build, the latest version is %s\n":                                    "Сборка не из релиза, последняя версия — %s\n",
	"The latest version is installed: %s\n":                                              "Установлена последняя версия: %s\n",
	"Already the latest version: %s\n":                                                   "Уже последняя версия: %s\n",
	"==> Installing %s\n":                                                                "==> Установка %s\n",
	"==> Updating %s → %s\n":                                                             "==> Обновление %s → %s\n",
	"Done: %s. The status line picks it up by itself, no need to restart Claude Code.\n": "Готово: %s. Строка статуса обновится сама, перезапускать Claude Code не нужно.\n",
	"could not download the checksums: %w":                                               "не удалось скачать контрольные суммы: %w",
	"release %s has no binary for %s/%s":                                                 "в релизе %s нет бинаря для %s/%s",
	"could not download %s: %w":                                                          "не удалось скачать %s: %w",
	"the checksum of %s does not match — the file is broken or tampered with":            "контрольная сумма %s не сошлась — файл побился или подменён",
	"could not write %s: %w":                                                             "не удалось записать %s: %w",
	"could not move the previous binary away: %w":                                        "не удалось убрать прежний бинарь: %w",
	"could not put the new binary in place: %w":                                          "не удалось поставить новый бинарь: %w",
	"could not find out the latest version: %w":                                          "не удалось узнать последнюю версию: %w",
	"could not parse the GitHub response: %w":                                            "не удалось разобрать ответ GitHub: %w",
	"%s has no releases yet":                                                             "у %s ещё нет релизов",
	"%s answered %s":                                                                     "%s ответил %s",

	// divoom: help and arguments
	`claudestatus divoom — limits panel on the screen of a Divoom Times Gate.

Usage:
  claudestatus divoom on           find the Times Gate on the network and turn the panel on
  claudestatus divoom off          turn the panel off and give the screen its clock face back
  claudestatus divoom              keep the panel updated (works while running)
  claudestatus divoom once         send the panel once and exit
  claudestatus divoom screen [N]   show or take over screen 1–5
  claudestatus divoom preview FILE save a frame to a file without touching the device

The settings are divoom.json in the application directory, created by on.
`: `claudestatus divoom — панель лимитов на экране Divoom Times Gate.

Использование:
  claudestatus divoom on           найти Times Gate в сети и включить панель
  claudestatus divoom off          выключить панель и вернуть экрану циферблат
  claudestatus divoom              держать панель обновлённой (работает, пока запущен)
  claudestatus divoom once         отправить панель один раз и выйти
  claudestatus divoom screen [N]   показать или занять экран 1–5
  claudestatus divoom preview FILE сохранить кадр в файл, не трогая устройство

Настройки — divoom.json в каталоге приложения, создаёт on.
`,
	"name a file: claudestatus divoom preview panel.gif": "укажите файл: claudestatus divoom preview panel.gif",
	"unknown command: %s\n\n%s":                          "неизвестная команда: %s\n\n%s",

	// divoom: device and bridge
	"Found device %s: %s\n": "Найдено устройство %s: %s\n",
	"could not determine our own address on the device network: %w": "не удалось определить свой адрес в сети устройства: %w",
	"the device did not take the frame within %s":                   "устройство не забрало кадр за %s",
	"Panel on screen %d of device %s, updated every %s\n":           "Панель на экране %d устройства %s, обновление каждые %s\n",
	"the device has been unreachable for more than %s, leaving":     "устройство недоступно дольше %s, выхожу",
	"the panel is not turned on — claudestatus divoom on":           "панель не включена — claudestatus divoom on",
	"%s does not parse: %w":                                         "%s не разбирается: %w",
	"the device is unreachable: %w":                                 "устройство недоступно: %w",
	"the device answered with something other than JSON: %w":        "устройство ответило не JSON: %w",
	"the device rejected the command: %s":                           "устройство отклонило команду: %s",
	"the device did not return its screen layout":                   "устройство не отдало раскладку экранов",
	"the previous clock face is unknown":                            "прежний циферблат неизвестен",
	"the device directory is unreachable: %w":                       "каталог устройств недоступен: %w",
	"no Divoom devices are visible on this network":                 "устройств Divoom в этой сети не видно",
	"could not open a port for the frames: %w":                      "не удалось открыть порт для кадров: %w",
	"The port is taken, serving frames on %d\n":                     "Порт занят, кадры отдаём на %d\n",
	"the bridge is already running":                                 "мост уже запущен",
	"The Divoom bridge is stopped":                                  "Остановлен мост Divoom",
	"The panel is on screen %d of %d\n":                             "Панель на экране %d из %d\n",
	"the screen is a number from 1 to %d, not %q":                   "экран — число от 1 до %d, а не %q",
	"The panel is on screen %d already\n":                           "Панель и так на экране %d\n",
	"The panel moved to screen %d\n":                                "Панель переехала на экран %d\n",
	"The panel is on, screen %d\n":                                  "Панель включена на экране %d\n",
	"The panel is off, the screen got its clock face back":          "Панель выключена, экрану возвращён его циферблат",
	"no snapshot found":                                             "снапшот не найден",
	"the snapshot is damaged":                                       "снапшот повреждён",
	"the snapshot holds no limits":                                  "лимитов в снапшоте нет",
	"could not encode the GIF: %w":                                  "не удалось закодировать GIF: %w",

	// divoom: labels drawn on the panel itself. Every character here must have
	// a glyph in font.go, and the string has to fit its bar at scale 2.
	"RESET": "СБРОС",
	"NO":    "НЕТ",
	"DATA":  "ДАННЫХ",
	"%dH":   "%dЧ",
	"%dM":   "%dМ",
}
