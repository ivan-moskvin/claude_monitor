//go:build !windows

package divoom

import (
	"os"
	"syscall"
	"unsafe"
)

// interactive — is there a terminal behind stdin, that is, is there anybody to
// answer a question. The mode of the file is not enough to go by: /dev/null is
// a character device just like a terminal, and a question asked into it only
// fails on the answer. Asking the terminal driver for its settings is the one
// check nothing else passes.
func interactive() bool {
	var settings syscall.Termios
	_, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, os.Stdin.Fd(),
		ioctlTermios, uintptr(unsafe.Pointer(&settings)), 0, 0, 0)
	return errno == 0
}
