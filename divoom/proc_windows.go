//go:build windows

package divoom

import (
	"os"
	"syscall"
)

// The Windows twin of proc_unix.go: there are no signals here, a process is
// finished the hard way, so the screen is restored not by the bridge itself but
// by whoever stops it.
const gracefulStop = false

const (
	detachedProcess     = 0x00000008
	createNewProcessGrp = 0x00000200
)

// detachAttrs starts the bridge without a console window and outside the
// process group of the caller — the same meaning Setsid has on Unix.
func detachAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: detachedProcess | createNewProcessGrp,
	}
}

// processAlive — on Windows FindProcess opens a handle to the process and
// returns an error when there is no such process any more.
func processAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = process.Release()
	return true
}

// terminate finishes the process: there is no gentle variant on Windows, so it
// coincides with forceKill, and cleaning up after the bridge is left to the
// caller.
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
