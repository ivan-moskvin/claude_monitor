// Package testenv cuts a test off from the machine it runs on.
//
// Everything the utility keeps lives in per-user directories it asks the OS
// for, and a test must touch neither the real settings of Claude Code nor the
// real config of the panel. Only tests import this package.
package testenv

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

// Home points the home, config and cache directories at a temporary one and
// returns it. Every variable of every platform is set at once — which of them
// the OS actually reads is not this helper's business — and the config and the
// cache get separate directories, exactly as they have on a real machine.
func Home(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	config := filepath.Join(home, "config")
	cache := filepath.Join(home, "cache")

	for name, value := range map[string]string{
		"HOME":            home,
		"USERPROFILE":     home,
		"XDG_CONFIG_HOME": config,
		"XDG_CACHE_HOME":  cache,
		"AppData":         config,
		"LocalAppData":    cache,
	} {
		t.Setenv(name, value)
	}
	return home
}

// CaptureStdout runs f with stdout redirected and returns what it printed. The
// commands talk to the user through stdout and nowhere else, so this is the
// only way to read what the user would see.
func CaptureStdout(t *testing.T, f func()) string {
	t.Helper()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	saved := os.Stdout
	os.Stdout = writer

	// Read in the background: a pipe holds 64 KiB, and a test that overflows it
	// would otherwise deadlock instead of failing.
	printed := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(reader)
		printed <- string(data)
	}()

	func() {
		defer func() {
			os.Stdout = saved
			_ = writer.Close()
		}()
		f()
	}()

	output := <-printed
	_ = reader.Close()
	return output
}
