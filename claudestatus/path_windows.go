//go:build windows

package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ivan-moskvin/claudestatus/i18n"
)

// samePathEntry compares an entry of PATH against a directory. Windows writes
// PATH as it pleases: another case, a trailing separator, an entry in quotes —
// all of them are the same directory, and a literal comparison would warn about
// a directory that is in PATH already.
func samePathEntry(entry, dir string) bool {
	entry = strings.Trim(entry, `"`)
	if entry == "" {
		return false
	}
	return strings.EqualFold(filepath.Clean(entry), filepath.Clean(dir))
}

// pathHint — how to put the directory into PATH so that it stays there. Not
// setx: it truncates the value at 1024 characters and would silently eat a PATH
// that grew past that.
func pathHint(dir string) string {
	return fmt.Sprintf(i18n.T("Command for PowerShell:  [Environment]::SetEnvironmentVariable(\"Path\", [Environment]::GetEnvironmentVariable(\"Path\", \"User\") + \";%s\", \"User\")\n"), dir)
}
