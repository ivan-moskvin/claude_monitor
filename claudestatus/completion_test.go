package main

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/ivan-moskvin/claude_monitor/i18n"
	"github.com/ivan-moskvin/claude_monitor/internal/testenv"
)

func TestCompletionScripts(t *testing.T) {
	cases := []struct {
		shell    string
		contains string
	}{
		{"zsh", "compdef _claudestatus claudestatus"},
		{"bash", "complete -F _claudestatus claudestatus"},
	}

	for _, c := range cases {
		t.Run(c.shell, func(t *testing.T) {
			script := testenv.CaptureStdout(t, func() {
				if err := completion([]string{c.shell}); err != nil {
					t.Fatalf("completion: %v", err)
				}
			})
			if !strings.Contains(script, c.contains) {
				t.Errorf("the %s script does not register itself: %q", c.shell, script)
			}
		})
	}
}

// The two shells complete the same utility: a command added to one script and
// forgotten in the other is a command half the users never see suggested.
func TestCompletionScriptsAgreeOnTheCommands(t *testing.T) {
	zsh := zshCommands(zshCompletion)
	bash := bashCommands(bashCompletion)

	if len(zsh) == 0 || len(bash) == 0 {
		t.Fatalf("no commands found: zsh %v, bash %v", zsh, bash)
	}
	if strings.Join(zsh, " ") != strings.Join(bash, " ") {
		t.Errorf("zsh completes %v, bash completes %v", zsh, bash)
	}
}

// The translated script is the one zsh actually gets: a broken translation
// would leave the shell with a script that does not define the commands.
func TestTranslatedCompletionKeepsTheCommands(t *testing.T) {
	source := zshCommands(zshCompletion)

	for _, lang := range i18n.Langs() {
		translated := zshCommands(i18n.In(lang, zshCompletion))
		if strings.Join(translated, " ") != strings.Join(source, " ") {
			t.Errorf("%s: the script completes %v; want %v", lang, translated, source)
		}
	}
}

// zshCommands reads the names out of the 'name:description' entries.
var zshEntry = regexp.MustCompile(`'([a-z]+):`)

func zshCommands(script string) []string {
	var names []string
	for _, match := range zshEntry.FindAllStringSubmatch(script, -1) {
		names = append(names, match[1])
	}
	sort.Strings(names)
	return names
}

// bashCommands reads the names out of the two space-separated lists.
var bashList = regexp.MustCompile(`commands="([^"]+)"`)

func bashCommands(script string) []string {
	var names []string
	for _, match := range bashList.FindAllStringSubmatch(script, -1) {
		names = append(names, strings.Fields(match[1])...)
	}
	sort.Strings(names)
	return names
}

func TestHasFlag(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{nil, false},
		{[]string{"--quiet"}, true},
		{[]string{"check", "--quiet"}, true},
		{[]string{"--quiet=1"}, false},
		{[]string{"-quiet"}, false},
		{[]string{"--loud"}, false},
	}

	for _, c := range cases {
		if got := hasFlag(c.args, "--quiet"); got != c.want {
			t.Errorf("hasFlag(%v, \"--quiet\") = %t; want %t", c.args, got, c.want)
		}
	}
}
