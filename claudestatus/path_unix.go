//go:build !windows

package main

import (
	"fmt"
	"path/filepath"

	"github.com/ivan-moskvin/claudestatus/i18n"
)

// samePathEntry compares an entry of PATH against a directory.
func samePathEntry(entry, dir string) bool {
	return filepath.Clean(entry) == filepath.Clean(dir)
}

// pathHint — how to put the directory into PATH so that it stays there.
func pathHint(dir string) string {
	return fmt.Sprintf(i18n.T("Line for ~/.zshrc:  export PATH=\"%s:$PATH\"\n"), dir)
}
