// Command claudestatus — the Claude limits status line, and its own upkeep.
//
// With no arguments it behaves as a statusLine command for Claude Code: the
// session JSON arrives on stdin, the status line goes to stdout. Subcommands
// exist for the human only: register in the settings, check for an update and
// install it.
package main

import (
	"fmt"
	"os"

	"github.com/ivan-moskvin/claude_monitor/divoom"
	"github.com/ivan-moskvin/claude_monitor/i18n"
)

// The version is read from the binary itself (see version): a build made
// outside a tag has no number, so the update mark never lights up for it.
var devVersion = i18n.T("built from source")

const usage = `claudestatus — Claude limits in the Claude Code status line.

Usage:
  claudestatus            status line: session JSON on stdin, line on stdout
  claudestatus install    register in ~/.claude/settings.json
  claudestatus check      check whether a new version is out
  claudestatus update     download the latest version and replace itself
  claudestatus uninstall  remove the status line, the cache and the binary
  claudestatus divoom     limits panel on a Divoom Times Gate (divoom help)
  claudestatus completion shell: completion script (zsh|bash)
  claudestatus version    print the version
  claudestatus help       this help

Environment:
  CLAUDESTATUS_LANG=ru|en         force the interface language
  CLAUDESTATUS_NO_AUTO_UPDATE=1   do not check for updates in the background
`

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		statusline()
		return
	}

	// --install is still understood: old install scripts spell it that way.
	switch args[0] {
	case "install", "--install":
		exit(installSelf())
	case "check":
		exit(check(hasFlag(args[1:], "--quiet")))
	case "update":
		exit(update())
	case "uninstall":
		exit(uninstall())
	case "divoom":
		exit(divoom.Run(args[1:]))
	case "completion":
		exit(completion(args[1:]))
	case "version", "--version", "-v":
		fmt.Println(version())
	case "help", "--help", "-h":
		fmt.Print(i18n.T(usage))
	default:
		fmt.Fprintf(os.Stderr, i18n.T("Unknown command: %s\n\n%s"), args[0], i18n.T(usage))
		os.Exit(2)
	}
}

func exit(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func hasFlag(args []string, name string) bool {
	for _, arg := range args {
		if arg == name {
			return true
		}
	}
	return false
}
