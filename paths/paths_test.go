package paths

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ivan-moskvin/claude_monitor/internal/testenv"
)

func TestDirIsCreated(t *testing.T) {
	testenv.Home(t)

	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("the application directory was not created: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("%s is not a directory", dir)
	}
	if filepath.Base(dir) != appName {
		t.Errorf("Dir() = %s; want a directory of our own", dir)
	}
}

func TestFileSitsInTheApplicationDirectory(t *testing.T) {
	testenv.Home(t)

	path, err := File("divoom.json")
	if err != nil {
		t.Fatalf("File: %v", err)
	}

	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if want := filepath.Join(dir, "divoom.json"); path != want {
		t.Errorf("File() = %s; want %s", path, want)
	}
}

// Files of earlier versions lived in ~/.claude: settings must not disappear
// because the utility decided to keep them somewhere else.
func TestFileMovesTheLegacyOneOver(t *testing.T) {
	home := testenv.Home(t)
	legacy := writeLegacy(t, home, "divoom.json", `{"ip":"192.168.0.5"}`)

	path, err := File("divoom.json")
	if err != nil {
		t.Fatalf("File: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the legacy file was not moved: %v", err)
	}
	if string(data) != `{"ip":"192.168.0.5"}` {
		t.Errorf("the moved file holds %q", data)
	}
	if _, err := os.Stat(legacy); err == nil {
		t.Error("the legacy file was copied instead of moved")
	}
}

// A file we already have wins: the one left in ~/.claude is older by definition.
func TestFileKeepsTheCurrentOne(t *testing.T) {
	home := testenv.Home(t)

	path, err := File("divoom.json")
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	if err := os.WriteFile(path, []byte("current"), 0o600); err != nil {
		t.Fatalf("writing the current file: %v", err)
	}
	writeLegacy(t, home, "divoom.json", "legacy")

	if _, err := File("divoom.json"); err != nil {
		t.Fatalf("File: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the file: %v", err)
	}
	if string(data) != "current" {
		t.Errorf("the file holds %q; want the current one to survive", data)
	}
}

func writeLegacy(t *testing.T, home, name, content string) string {
	t.Helper()

	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating the legacy directory: %v", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing the legacy file: %v", err)
	}
	return path
}
