package main

import (
	"os"
	"testing"
)

// feedStdin hands the session JSON to the status line the way Claude Code does.
func feedStdin(t *testing.T, input string) {
	t.Helper()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	if _, err := writer.WriteString(input); err != nil {
		t.Fatalf("writing to stdin: %v", err)
	}
	_ = writer.Close()

	saved := os.Stdin
	os.Stdin = reader
	t.Cleanup(func() {
		os.Stdin = saved
		_ = reader.Close()
	})
}

// withVersion pretends the binary was built from a release tag: version reads
// versionOverride, which CI stamps in with -ldflags.
func withVersion(t *testing.T, tag string) {
	t.Helper()

	saved := versionOverride
	versionOverride = tag
	t.Cleanup(func() { versionOverride = saved })
}
