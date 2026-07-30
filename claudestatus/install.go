package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ivan-moskvin/claude_monitor/divoom"
	"github.com/ivan-moskvin/claude_monitor/i18n"
	"github.com/ivan-moskvin/claude_monitor/paths"
)

// installSelf registers the binary that is running right now.
func installSelf() error {
	exe, err := selfPath()
	if err != nil {
		return err
	}
	return install(exe)
}

// install writes the exe command into statusLine of ~/.claude/settings.json.
// The path is absolute: the status line is invoked from any directory.
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
				return fmt.Errorf(i18n.T("%s does not parse — fix it by hand"), path)
			}
		}
		// Back up before writing and only when the file exists — otherwise we
		// would overwrite somebody's settings with no way back.
		if err := os.WriteFile(path+".bak", data, 0o644); err != nil {
			return fmt.Errorf(i18n.T("could not back up settings.json: %w"), err)
		}
	case !os.IsNotExist(err):
		return fmt.Errorf(i18n.T("could not read settings.json: %w"), err)
	}

	// Quoted, in case the path to the repository has spaces in it.
	command := fmt.Sprintf("%q", exe)
	if previous, ok := settings["statusLine"].(map[string]any); ok {
		if was, _ := previous["command"].(string); was != "" && was != command {
			fmt.Printf(i18n.T("Replacing the previous status line: %s\n"), was)
			fmt.Printf(i18n.T("The previous settings are in %s.bak\n"), path)
		}
	}

	settings["statusLine"] = map[string]string{"type": "command", "command": command}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf(i18n.T("could not create %s: %w"), filepath.Dir(path), err)
	}

	updated, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(updated, '\n'), 0o644); err != nil {
		return err
	}

	fmt.Printf(i18n.T("Status line registered in %s\n"), path)
	warnIfNotInPath(exe)
	return nil
}

// warnIfNotInPath. Claude Code calls the binary by its absolute path and does
// fine without PATH, but the user types claudestatus by hand — and go install
// drops the binary where not everybody has a PATH entry.
func warnIfNotInPath(exe string) {
	dir := filepath.Dir(exe)
	for _, entry := range filepath.SplitList(os.Getenv("PATH")) {
		if samePathEntry(entry, dir) {
			return
		}
	}
	fmt.Printf(i18n.T("\n%s is not in PATH — the claudestatus command will not be found.\n"), dir)
	fmt.Print(pathHint(dir))
}

// uninstall clears everything behind us: the settings entry, the check cache
// and the binary. The order matters — settings first, or Claude Code manages to
// call an already deleted file.
func uninstall() error {
	exe, err := selfPath()
	if err != nil {
		return err
	}

	if err := removeFromSettings(exe); err != nil {
		return err
	}

	// The bridge is a separate process and outlives the deleted binary unless
	// it is stopped: the panel stays on the device with nobody left to update
	// it.
	divoom.Stop()

	// The whole application directory: usage snapshot, bridge settings, pid file.
	if dir, err := paths.Dir(); err == nil {
		if err := os.RemoveAll(dir); err == nil {
			fmt.Printf(i18n.T("Removed %s\n"), dir)
		}
	}

	if dir, err := cacheDir(); err == nil {
		if err := os.RemoveAll(dir); err == nil {
			fmt.Printf(i18n.T("Removed the cache %s\n"), dir)
		}
	}

	// A running binary can be deleted in place everywhere but on Windows.
	if err := os.Remove(exe); err != nil {
		fmt.Printf(i18n.T("The binary is still there — remove it by hand: %s\n"), exe)
		return nil
	}
	fmt.Printf(i18n.T("Removed the binary %s\n"), exe)
	return nil
}

// removeFromSettings takes our status line out of settings.json. Somebody
// else's is left alone: replacing it was the user's decision, and we have
// nothing to put back.
func removeFromSettings(exe string) error {
	path, err := settingsPath()
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		fmt.Printf(i18n.T("There is no %s — nothing to clean in the settings\n"), path)
		return nil
	}
	if err != nil {
		return fmt.Errorf(i18n.T("could not read settings.json: %w"), err)
	}

	settings := map[string]any{}
	if len(bytes.TrimSpace(data)) > 0 {
		if json.Unmarshal(data, &settings) != nil {
			return fmt.Errorf(i18n.T("%s does not parse — fix it by hand"), path)
		}
	}

	current, ok := settings["statusLine"].(map[string]any)
	if !ok {
		fmt.Printf(i18n.T("There is no status line in %s\n"), path)
		return nil
	}
	if command, _ := current["command"].(string); command != fmt.Sprintf("%q", exe) {
		fmt.Printf(i18n.T("%s holds a status line that is not ours — leaving it as is:\n  %s\n"), path, command)
		return nil
	}

	if err := os.WriteFile(path+".bak", data, 0o644); err != nil {
		return fmt.Errorf(i18n.T("could not back up settings.json: %w"), err)
	}
	delete(settings, "statusLine")

	updated, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(updated, '\n'), 0o644); err != nil {
		return err
	}

	fmt.Printf(i18n.T("Status line removed from %s (the previous settings are in %s.bak)\n"), path, path)
	return nil
}

func settingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

// selfPath — the path to our own binary with symlinks resolved: the command is
// run through a link from ~/.local/bin, but it has to update the real file.
func selfPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf(i18n.T("could not determine our own path: %w"), err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf(i18n.T("could not determine our own path: %w"), err)
	}
	return exe, nil
}
