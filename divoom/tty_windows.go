//go:build windows

package divoom

import (
	"os"
	"syscall"
	"unsafe"
)

// The Windows twin of tty_unix.go: a console handle is the one thing
// GetConsoleMode answers for, so a redirected stdin fails it.
var procGetConsoleMode = syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleMode")

func interactive() bool {
	var mode uint32
	result, _, _ := procGetConsoleMode.Call(os.Stdin.Fd(), uintptr(unsafe.Pointer(&mode)))
	return result != 0
}
