package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/ivan-moskvin/claude_monitor/paths"
)

// The usage snapshot is the only contract with whoever shows the numbers
// outside the status line (the panel on a Divoom Times Gate). The rate_limits
// block is copied as it came: the writer picks no fields, so new windows appear
// on their own.
const snapshotName = "usage-snapshot.json"

func saveSnapshot(limits map[string]json.RawMessage) error {
	if len(limits) == 0 {
		// Claude Code sometimes calls the status line without any limits — the
		// previous numbers are more honest than empty ones.
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

	// Written to a temporary file next to it and renamed: the reader never sees
	// half a record and needs no locking.
	temporary := filepath.Join(dir, snapshotName+".tmp")
	if err := os.WriteFile(temporary, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, filepath.Join(dir, snapshotName))
}
