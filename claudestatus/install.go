package main

import (
	"bytes"
	"encoding/json"
	"fmt"
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
