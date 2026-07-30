//go:build linux

package divoom

import "syscall"

// The Linux name of the request that reads the terminal settings.
const ioctlTermios = syscall.TCGETS
