package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/ivan-moskvin/claude_monitor/divoom"
	"os"
	"path/filepath"
)

// installSelf прописывает в настройки бинарь, который сейчас выполняется.
func installSelf() error {
	exe, err := selfPath()
	if err != nil {
		return err
	}
	return install(exe)
}

// install прописывает команду exe в statusLine ~/.claude/settings.json.
// Путь берём абсолютный: строка статуса вызывается из любого каталога.
func install(exe string) error {
	path, err := settingsPath()
	if err != nil {
		return err
	}

	settings := map[string]any{}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if len(bytes.TrimSpace(data)) > 0 {
			if json.Unmarshal(data, &settings) != nil {
				return fmt.Errorf("%s не разбирается — поправьте его вручную", path)
			}
		}
		// Бэкап до записи и только при существующем файле —
		// иначе затрём чужие настройки без возможности откатиться.
		if err := os.WriteFile(path+".bak", data, 0o644); err != nil {
			return fmt.Errorf("не удалось сделать бэкап settings.json: %w", err)
		}
	case !os.IsNotExist(err):
		return fmt.Errorf("не удалось прочитать settings.json: %w", err)
	}

	// Кавычки — на случай пробелов в пути к репозиторию.
	command := fmt.Sprintf("%q", exe)
	if previous, ok := settings["statusLine"].(map[string]any); ok {
		if was, _ := previous["command"].(string); was != "" && was != command {
			fmt.Printf("Заменяю прежнюю строку статуса: %s\n", was)
			fmt.Printf("Прежние настройки — в %s.bak\n", path)
		}
	}

	settings["statusLine"] = map[string]string{"type": "command", "command": command}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("не удалось создать %s: %w", filepath.Dir(path), err)
	}

	updated, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(updated, '\n'), 0o644); err != nil {
		return err
	}

	fmt.Printf("Строка статуса прописана в %s\n", path)
	warnIfNotInPath(exe)
	return nil
}

// warnIfNotInPath. Claude Code зовёт бинарь по абсолютному пути и без PATH
// обойдётся, а вот сам пользователь набирает claudestatus руками — go install
// же кладёт бинарь туда, где PATH бывает не у всех.
func warnIfNotInPath(exe string) {
	dir := filepath.Dir(exe)
	for _, entry := range filepath.SplitList(os.Getenv("PATH")) {
		if entry == dir {
			return
		}
	}
	fmt.Printf("\nКаталог %s не в PATH — команда claudestatus не найдётся.\n", dir)
	fmt.Printf("Строка для ~/.zshrc:  export PATH=\"%s:$PATH\"\n", dir)
}

// uninstall убирает за собой всё: запись в настройках, кэш проверок и бинарь.
// Порядок важен — сначала настройки, иначе Claude Code успеет позвать удалённый файл.
func uninstall() error {
	exe, err := selfPath()
	if err != nil {
		return err
	}

	if err := removeFromSettings(exe); err != nil {
		return err
	}

	// Мост живёт отдельным процессом и переживёт удаление бинаря, если его не
	// остановить: панель на устройстве останется, а обновлять её будет некому.
	divoom.Stop()

	if dir, err := cacheDir(); err == nil {
		if err := os.RemoveAll(dir); err == nil {
			fmt.Printf("Удалён кэш %s\n", dir)
		}
	}

	// Запущенный бинарь удаляется на месте везде, кроме Windows.
	if err := os.Remove(exe); err != nil {
		fmt.Printf("Бинарь остался — удалите вручную: %s\n", exe)
		return nil
	}
	fmt.Printf("Удалён бинарь %s\n", exe)
	return nil
}

// removeFromSettings вынимает из settings.json нашу строку статуса. Чужую
// не трогаем: заменить её было решением пользователя, а вернуть нам нечего.
func removeFromSettings(exe string) error {
	path, err := settingsPath()
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		fmt.Printf("%s нет — настройки чистить не нужно\n", path)
		return nil
	}
	if err != nil {
		return fmt.Errorf("не удалось прочитать settings.json: %w", err)
	}

	settings := map[string]any{}
	if len(bytes.TrimSpace(data)) > 0 {
		if json.Unmarshal(data, &settings) != nil {
			return fmt.Errorf("%s не разбирается — поправьте его вручную", path)
		}
	}

	current, ok := settings["statusLine"].(map[string]any)
	if !ok {
		fmt.Printf("В %s строки статуса нет\n", path)
		return nil
	}
	if command, _ := current["command"].(string); command != fmt.Sprintf("%q", exe) {
		fmt.Printf("В %s прописана не наша строка статуса — оставляю как есть:\n  %s\n", path, command)
		return nil
	}

	if err := os.WriteFile(path+".bak", data, 0o644); err != nil {
		return fmt.Errorf("не удалось сделать бэкап settings.json: %w", err)
	}
	delete(settings, "statusLine")

	updated, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(updated, '\n'), 0o644); err != nil {
		return err
	}

	fmt.Printf("Строка статуса убрана из %s (прежние настройки — в %s.bak)\n", path, path)
	return nil
}

func settingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

// selfPath — путь к собственному бинарю с раскрытыми симлинками: команду
// запускают через ссылку из ~/.local/bin, а обновляться нужно в клоне.
func selfPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("не удалось определить свой путь: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("не удалось определить свой путь: %w", err)
	}
	return exe, nil
}
