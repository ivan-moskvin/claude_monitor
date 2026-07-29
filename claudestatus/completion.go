package main

import (
	"fmt"
	"os"

	"github.com/ivan-moskvin/claude_monitor/i18n"
)

// Command completion: without it the shell only suggests what came out of the
// history, that is only what has already been typed by hand.
const completionUsage = `Usage:
  claudestatus completion zsh   >> ~/.zshrc
  claudestatus completion bash  >> ~/.bashrc
`

const zshCompletion = `_claudestatus() {
  local -a commands
  commands=(
    'install:register in the Claude Code settings'
    'check:check whether a new version is out'
    'update:update to the latest version'
    'uninstall:remove the status line and the binary'
    'divoom:limits panel on a Divoom Times Gate'
    'completion:shell completion script'
    'version:print the version'
    'help:help'
  )
  local -a divoom_commands
  divoom_commands=(
    'on:find the device and turn the panel on'
    'off:turn the panel off'
    'once:send the panel once'
    'screen:show or take over screen 1-5'
    'preview:save a frame to a file'
    'help:help'
  )

  if (( CURRENT == 2 )); then
    _describe 'command' commands
  elif (( CURRENT == 3 )) && [[ ${words[2]} == divoom ]]; then
    _describe 'command' divoom_commands
  fi
}
compdef _claudestatus claudestatus
`

const bashCompletion = `_claudestatus() {
  local commands="install check update uninstall divoom completion version help"
  local divoom_commands="on off once preview screen help"

  if [ "$COMP_CWORD" -eq 1 ]; then
    COMPREPLY=($(compgen -W "$commands" -- "${COMP_WORDS[1]}"))
  elif [ "$COMP_CWORD" -eq 2 ] && [ "${COMP_WORDS[1]}" = divoom ]; then
    COMPREPLY=($(compgen -W "$divoom_commands" -- "${COMP_WORDS[2]}"))
  fi
}
complete -F _claudestatus claudestatus
`

func completion(args []string) error {
	shell := ""
	if len(args) > 0 {
		shell = args[0]
	}

	switch shell {
	case "zsh":
		// Only the zsh script carries descriptions, so only it is translated;
		// bash completes bare command names.
		fmt.Print(i18n.T(zshCompletion))
	case "bash":
		fmt.Print(bashCompletion)
	default:
		fmt.Fprint(os.Stderr, i18n.T(completionUsage))
		os.Exit(2)
	}
	return nil
}
