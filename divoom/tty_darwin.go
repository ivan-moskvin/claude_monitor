//go:build darwin

package divoom

import "syscall"

// The BSD name of the request that reads the terminal settings.
const ioctlTermios = syscall.TIOCGETA
