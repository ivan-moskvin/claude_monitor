//go:build windows

package divoom

import (
	"os"
	"syscall"
)

// Windows-двойник proc_unix.go: сигналов здесь нет, процесс завершается
// жёстко, поэтому экран возвращает не сам мост, а тот, кто его останавливает.
const gracefulStop = false

const (
	detachedProcess     = 0x00000008
	createNewProcessGrp = 0x00000200
)

// detachAttrs запускает мост без консольного окна и вне группы процессов
// вызвавшего — тот же смысл, что у Setsid на Unix.
func detachAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: detachedProcess | createNewProcessGrp,
	}
}

// processAlive — на Windows FindProcess открывает описатель процесса и
// возвращает ошибку, если такого больше нет.
func processAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = process.Release()
	return true
}

// terminate завершает процесс: мягкого варианта на Windows нет, поэтому он
// совпадает с forceKill, а прибраться за мостом придётся вызывающему.
func terminate(pid int) error {
	return forceKill(pid)
}

func forceKill(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}
