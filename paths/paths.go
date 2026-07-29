// Пакет paths — где утилита держит свои файлы.
//
// Не в ~/.claude: тот каталог принадлежит Claude Code, и складывать туда чужое
// состояние — значит мусорить у него под ногами. Каждая система предлагает своё
// место для настроек приложения, его и берём.
package paths

import (
	"os"
	"path/filepath"
)

const appName = "claudestatus"

// Dir — каталог наших файлов, созданный, если его ещё нет:
// macOS ~/Library/Application Support/claudestatus, Windows %AppData%\claudestatus,
// Linux ~/.config/claudestatus.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, appName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// File — путь к нашему файлу. Если он ещё лежит в ~/.claude от прежних версий,
// переносим: пользователь не должен терять токен устройства из-за переезда.
func File(name string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		if home, err := os.UserHomeDir(); err == nil {
			legacy := filepath.Join(home, ".claude", name)
			if _, err := os.Stat(legacy); err == nil {
				_ = os.Rename(legacy, path)
			}
		}
	}
	return path, nil
}
