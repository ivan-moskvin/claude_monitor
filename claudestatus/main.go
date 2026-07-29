// Команда claudestatus — строка статуса с лимитами Claude и её же обслуживание.
//
// Без аргументов работает как statusLine-команда Claude Code: JSON сессии
// приходит на stdin, строка статуса уходит в stdout. Подкоманды нужны только
// человеку: прописать себя в настройки, проверить и поставить обновление.
package main

import (
	"fmt"
	"os"

	"github.com/ivan-moskvin/claude_monitor/divoom"
)

// Версия берётся из самого бинаря (см. version): у сборки не из тега номера
// нет, поэтому значок обновления для неё не зажигается.
const devVersion = "сборка из исходников"

const usage = `claudestatus — лимиты Claude в строке статуса Claude Code.

Использование:
  claudestatus            строка статуса: JSON сессии на stdin, строка на stdout
  claudestatus install    прописать себя в ~/.claude/settings.json
  claudestatus check      проверить, вышла ли новая версия
  claudestatus update     скачать последнюю версию и заменить себя ею
  claudestatus uninstall  убрать строку статуса, кэш и сам бинарь
  claudestatus divoom     показывать лимиты на экране Divoom Times Gate
  claudestatus version    показать версию
  claudestatus help       эта справка

Переменные окружения:
  CLAUDESTATUS_NO_AUTO_UPDATE=1   не проверять обновления в фоне
`

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		statusline()
		return
	}

	// --install понимаем по-прежнему: он прописан в старых скриптах установки.
	switch args[0] {
	case "install", "--install":
		exit(installSelf())
	case "check":
		exit(check(hasFlag(args[1:], "--quiet")))
	case "update":
		exit(update())
	case "uninstall":
		exit(uninstall())
	case "divoom":
		exit(divoom.Run(args[1:]))
	case "version", "--version", "-v":
		fmt.Println(version())
	case "help", "--help", "-h":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "Неизвестная команда: %s\n\n%s", args[0], usage)
		os.Exit(2)
	}
}

func exit(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func hasFlag(args []string, name string) bool {
	for _, arg := range args {
		if arg == name {
			return true
		}
	}
	return false
}
