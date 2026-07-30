package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ivan-moskvin/claude_monitor/internal/testenv"
)

const installedExe = "/usr/local/bin/claudestatus"

// The path is quoted: a directory with a space in it would otherwise split into
// a command and an argument.
const installedCommand = `"/usr/local/bin/claudestatus"`

func TestInstallIntoAFreshMachine(t *testing.T) {
	testenv.Home(t)

	testenv.CaptureStdout(t, func() {
		if err := install(installedExe); err != nil {
			t.Fatalf("install: %v", err)
		}
	})

	if got := statusLineCommand(t); got != installedCommand {
		t.Errorf("statusLine.command = %q; want %q", got, installedCommand)
	}
	if got := settings(t)["statusLine"].(map[string]any)["type"]; got != "command" {
		t.Errorf("statusLine.type = %v; want command", got)
	}
	// There was nothing to back up: a .bak of a file that never existed would
	// only be in the way.
	if _, err := os.Stat(settingsFile(t) + ".bak"); err == nil {
		t.Error("a backup was made of a settings file that did not exist")
	}
}

func TestInstallKeepsTheRestOfTheSettings(t *testing.T) {
	testenv.Home(t)
	writeSettings(t, `{
		"theme": "dark",
		"permissions": {"allow": ["Bash(go test:*)"]},
		"statusLine": {"type": "command", "command": "somebody-elses-line"}
	}`)

	output := testenv.CaptureStdout(t, func() {
		if err := install(installedExe); err != nil {
			t.Fatalf("install: %v", err)
		}
	})

	current := settings(t)
	if current["theme"] != "dark" {
		t.Errorf("theme = %v; want the setting to survive", current["theme"])
	}
	if _, ok := current["permissions"]; !ok {
		t.Error("permissions disappeared")
	}
	if got := statusLineCommand(t); got != installedCommand {
		t.Errorf("statusLine.command = %q; want %q", got, installedCommand)
	}

	// Overwriting somebody else's status line is said out loud, and the old one
	// is recoverable from the backup.
	if !strings.Contains(output, "somebody-elses-line") {
		t.Errorf("install said %q; want the previous command in it", output)
	}
	if backup := readFile(t, settingsFile(t)+".bak"); !strings.Contains(backup, "somebody-elses-line") {
		t.Errorf("the backup holds %q; want the settings as they were", backup)
	}
}

// Settings we cannot read we do not touch: rewriting them from scratch would
// throw away everything the user has configured.
func TestInstallRefusesBrokenSettings(t *testing.T) {
	testenv.Home(t)
	const broken = `{"theme": "dark",`
	writeSettings(t, broken)

	err := install(installedExe)
	if err == nil {
		t.Fatal("install rewrote a settings file it could not parse")
	}
	if got := readFile(t, settingsFile(t)); got != broken {
		t.Errorf("the settings file was changed anyway: %q", got)
	}
}

// An empty file is not broken settings — it is a file somebody created and
// never filled in.
func TestInstallOverAnEmptyFile(t *testing.T) {
	testenv.Home(t)
	writeSettings(t, "\n")

	testenv.CaptureStdout(t, func() {
		if err := install(installedExe); err != nil {
			t.Fatalf("install: %v", err)
		}
	})
	if got := statusLineCommand(t); got != installedCommand {
		t.Errorf("statusLine.command = %q; want %q", got, installedCommand)
	}
}

func TestRemoveFromSettingsTakesOutOurOwnLine(t *testing.T) {
	testenv.Home(t)
	writeSettings(t, `{"theme":"dark","statusLine":{"type":"command","command":`+quoteJSON(installedCommand)+`}}`)

	testenv.CaptureStdout(t, func() {
		if err := removeFromSettings(installedExe); err != nil {
			t.Fatalf("removeFromSettings: %v", err)
		}
	})

	current := settings(t)
	if _, ok := current["statusLine"]; ok {
		t.Error("the status line stayed in the settings")
	}
	if current["theme"] != "dark" {
		t.Errorf("theme = %v; want the rest of the settings untouched", current["theme"])
	}
	if backup := readFile(t, settingsFile(t)+".bak"); !strings.Contains(backup, "statusLine") {
		t.Error("no backup was left of the settings we edited")
	}
}

// Somebody else's status line is left alone: replacing ours with it was the
// user's decision, and we have nothing to put back in its place.
func TestRemoveFromSettingsLeavesAForeignLine(t *testing.T) {
	testenv.Home(t)
	const foreign = `{"statusLine":{"type":"command","command":"somebody-elses-line"}}`
	writeSettings(t, foreign)

	testenv.CaptureStdout(t, func() {
		if err := removeFromSettings(installedExe); err != nil {
			t.Fatalf("removeFromSettings: %v", err)
		}
	})

	if got := readFile(t, settingsFile(t)); got != foreign {
		t.Errorf("the settings were changed: %q", got)
	}
	if _, err := os.Stat(settingsFile(t) + ".bak"); err == nil {
		t.Error("a backup was made of settings that were not touched")
	}
}

func TestRemoveFromSettingsWithNothingToRemove(t *testing.T) {
	cases := map[string]string{
		"no settings file at all": "",
		"no status line in it":    `{"theme":"dark"}`,
	}

	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			testenv.Home(t)
			if content != "" {
				writeSettings(t, content)
			}

			testenv.CaptureStdout(t, func() {
				// Nothing to do is not a failure: uninstall runs this on
				// machines that never had the status line registered.
				if err := removeFromSettings(installedExe); err != nil {
					t.Errorf("removeFromSettings: %v", err)
				}
			})
		})
	}
}

func TestRemoveFromSettingsRefusesBrokenSettings(t *testing.T) {
	testenv.Home(t)
	const broken = `{"statusLine":`
	writeSettings(t, broken)

	if err := removeFromSettings(installedExe); err == nil {
		t.Fatal("removeFromSettings edited a settings file it could not parse")
	}
	if got := readFile(t, settingsFile(t)); got != broken {
		t.Errorf("the settings file was changed anyway: %q", got)
	}
}

// install writes a file Claude Code has to read back: indented JSON with a
// trailing newline, not a single line of it.
func TestInstallWritesReadableJSON(t *testing.T) {
	testenv.Home(t)
	testenv.CaptureStdout(t, func() {
		if err := install(installedExe); err != nil {
			t.Fatalf("install: %v", err)
		}
	})

	written := readFile(t, settingsFile(t))
	if !strings.HasSuffix(written, "\n") {
		t.Error("the settings file has no trailing newline")
	}
	if !strings.Contains(written, "\n  ") {
		t.Errorf("the settings file is not indented: %q", written)
	}
}

// The warning about PATH is the last thing the user reads after an install, and
// a false one sends them editing PATH for a directory that is in it already.
func TestWarnIfNotInPath(t *testing.T) {
	dir := filepath.Dir(installedExe)
	elsewhere := filepath.Join("elsewhere", "bin")
	list := func(entries ...string) string {
		return strings.Join(entries, string(filepath.ListSeparator))
	}

	cases := map[string]struct {
		path string
		warn bool
	}{
		"the directory alone":     {path: dir},
		"among other entries":     {path: list(elsewhere, dir, "")},
		"with a trailing slash":   {path: dir + string(filepath.Separator)},
		"nowhere in PATH":         {path: elsewhere, warn: true},
		"PATH is not set at all":  {path: "", warn: true},
		"only a longer directory": {path: filepath.Join(dir, "deeper"), warn: true},
	}

	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("PATH", want.path)

			output := testenv.CaptureStdout(t, func() { warnIfNotInPath(installedExe) })
			if warned := output != ""; warned != want.warn {
				t.Errorf("PATH=%q printed %q; want a warning: %v", want.path, output, want.warn)
			}
			// The user is told what to do about it, not only that something is
			// wrong.
			if want.warn && !strings.Contains(output, dir) {
				t.Errorf("the warning %q does not say which directory to add", output)
			}
		})
	}
}

func settingsFile(t *testing.T) string {
	t.Helper()

	path, err := settingsPath()
	if err != nil {
		t.Fatalf("settingsPath: %v", err)
	}
	return path
}

func writeSettings(t *testing.T, content string) {
	t.Helper()

	path := settingsFile(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating the settings directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing the settings: %v", err)
	}
}

func settings(t *testing.T) map[string]any {
	t.Helper()

	var parsed map[string]any
	if err := json.Unmarshal([]byte(readFile(t, settingsFile(t))), &parsed); err != nil {
		t.Fatalf("the settings do not parse: %v", err)
	}
	return parsed
}

func statusLineCommand(t *testing.T) string {
	t.Helper()

	line, ok := settings(t)["statusLine"].(map[string]any)
	if !ok {
		t.Fatal("there is no status line in the settings")
	}
	command, _ := line["command"].(string)
	return command
}

// quoteJSON quotes a string for embedding in the JSON of a test fixture.
func quoteJSON(value string) string {
	quoted, _ := json.Marshal(value)
	return string(quoted)
}
