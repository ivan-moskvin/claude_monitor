//go:build !windows

package divoom

import "syscall"

// Dealing with somebody else's process works differently on different systems,
// so it lives in files behind build tags: the syscall package on Windows has
// neither Kill nor the Setsid field, and cross-compiling the release binaries
// used to fail on exactly that.

// gracefulStop — can the system ask a process to finish and let it clean up. On
// Unix that is SIGTERM, and the bridge manages to give the screen its clock
// face back.
const gracefulStop = true

// detachAttrs detaches the bridge from the process group of Claude Code:
// otherwise it dies together with the status line call that started it.
func detachAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

// processAlive — does the process exist. Signal 0 does nothing to it but
// answers that question.
func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func terminate(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
}

func forceKill(pid int) error {
	return syscall.Kill(pid, syscall.SIGKILL)
}
