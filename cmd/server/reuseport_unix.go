//go:build !windows

package main

import (
	"syscall"

	"golang.org/x/sys/unix"
)

func setReusePort(fd uintptr) error {
	return syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, unix.SO_REUSEPORT, 1)
}
