//go:build windows

package main

import (
	"strings"
	"testing"
)

// Windows writes PATH as it pleases, and every one of these is the directory
// install.ps1 has just put the binary into: a literal comparison would warn
// about a PATH that is already correct.
func TestSamePathEntryOnWindows(t *testing.T) {
	const dir = `C:\Users\rockman\AppData\Local\claudestatus`

	same := []string{
		dir,
		`c:\users\rockman\appdata\local\claudestatus`,
		dir + `\`,
		`"` + dir + `"`,
		`C:\Users\rockman\AppData\Local\..\Local\claudestatus`,
	}
	for _, entry := range same {
		if !samePathEntry(entry, dir) {
			t.Errorf("samePathEntry(%q, %q) = false; want the same directory", entry, dir)
		}
	}

	other := []string{"", `"`, `C:\Users\rockman\AppData\Local`, dir + `\bin`}
	for _, entry := range other {
		if samePathEntry(entry, dir) {
			t.Errorf("samePathEntry(%q, %q) = true; want another directory", entry, dir)
		}
	}
}

// The hint has to be runnable where it is printed: a ~/.zshrc line helps nobody
// in PowerShell.
func TestPathHintOnWindows(t *testing.T) {
	const dir = `C:\Users\rockman\AppData\Local\claudestatus`

	hint := pathHint(dir)
	if !strings.Contains(hint, dir) {
		t.Errorf("the hint %q does not name the directory", hint)
	}
	if strings.Contains(hint, "zshrc") || strings.Contains(hint, "export PATH") {
		t.Errorf("the hint %q is for another system's shell", hint)
	}
}
