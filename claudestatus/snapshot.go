package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/ivan-moskvin/claude_monitor/paths"
	"time"
)

// Снапшот лимитов — единственный контракт с теми, кто показывает расход вне
// строки статуса (панель на Divoom Times Gate). Блок rate_limits копируется
// как пришёл: writer не выбирает поля, поэтому новые окна появятся сами.
const snapshotName = "usage-snapshot.json"

func saveSnapshot(limits map[string]json.RawMessage) error {
	if len(limits) == 0 {
		// Claude Code иногда зовёт строку статуса без лимитов — прежние
		// цифры честнее пустых.
		return nil
	}

	dir, err := paths.Dir()
	if err != nil {
		return err
	}

	data, err := json.Marshal(struct {
		RateLimits map[string]json.RawMessage `json:"rate_limits"`
		UpdatedAt  string                     `json:"updated_at"`
	}{limits, time.Now().UTC().Format(time.RFC3339)})
	if err != nil {
		return err
	}

	// Пишем через временный файл рядом и переименовываем: читатель никогда
	// не увидит половину записи и может читать без блокировок.
	temporary := filepath.Join(dir, snapshotName+".tmp")
	if err := os.WriteFile(temporary, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(temporary, filepath.Join(dir, snapshotName))
}
