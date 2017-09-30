package main

import (
	"syscall"
	"unsafe"
)

func (e *editor) getShellNormal() syscall.Errno {
	_, _, err := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(e.reader), syscall.TCGETS, uintptr(unsafe.Pointer(&e.orignial)), 0, 0, 0)
	return err
}
