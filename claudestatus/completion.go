package main

import (
	"fmt"
	"os"
)

// Автодополнение команд: без него в оболочке всплывают только подсказки из
// истории, то есть лишь то, что уже набирали руками.
const completionUsage = `Использование:
  claudestatus completion zsh   >> ~/.zshrc
  claudestatus completion bash  >> ~/.bashrc
`

const zshCompletion = `_claudestatus() {
  local -a commands
  commands=(
    'install:прописать себя в настройки Claude Code'
    'check:проверить, вышла ли новая версия'
    'update:обновиться до последней версии'
    'uninstall:убрать строку статуса и сам бинарь'
    'divoom:панель лимитов на Divoom Times Gate'
    'completion:скрипт автодополнения для оболочки'
    'version:показать версию'
    'help:справка'
  )
  local -a divoom_commands
  divoom_commands=(
    'on:найти устройство и включить панель'
    'off:выключить панель'
    'once:отправить панель один раз'
    'screen:показать или занять экран 1-5'
    'preview:сохранить кадр в файл'
    'help:справка'
  )

  if (( CURRENT == 2 )); then
    _describe 'команда' commands
  elif (( CURRENT == 3 )) && [[ ${words[2]} == divoom ]]; then
    _describe 'команда' divoom_commands
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
		fmt.Print(zshCompletion)
	case "bash":
		fmt.Print(bashCompletion)
	default:
		fmt.Fprint(os.Stderr, completionUsage)
		os.Exit(2)
	}
	return nil
}
