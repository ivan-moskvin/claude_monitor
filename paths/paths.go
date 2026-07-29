// Package paths — where the utility keeps its files.
//
// Not in ~/.claude: that directory belongs to Claude Code, and putting foreign
// state there means littering under its feet. Every system offers its own place
// for application settings, and that is the one we take.
package paths

import (
	"os"
	"path/filepath"
)

const appName = "claudestatus"

// Dir — the directory of our files, created if it is not there yet:
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

// File — the path to one of our files. Files of earlier versions are moved out
// of ~/.claude: settings must not disappear because of a relocation.
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
