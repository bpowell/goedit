package main

import (
	"syscall"
	"unsafe"
)

func (e *editor) getShellNormal() syscall.Errno {
	_, _, err := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(e.reader), syscall.TCGETS, uintptr(unsafe.Pointer(&e.orignial)), 0, 0, 0)
	return err
}

func (e *editor) setWindowSize() syscall.Errno {
	winsize := winsize{}
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, uintptr(e.reader), syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&winsize))); err != 0 {
		return err
	}

	e.height = int(winsize.height) - 2
	e.width = int(winsize.width)

	return 0
}

func (e *editor) rawMode(argp syscall.Termios) syscall.Errno {
	_, _, err := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(e.reader), 0x5404, uintptr(unsafe.Pointer(&argp)), 0, 0, 0)
	return err
}

func (e *editor) resetMode() {
	_, _, err := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(e.reader), 0x5404, uintptr(unsafe.Pointer(&e.orignial)), 0, 0, 0)
	if err != 0 {
		logger.Fatal(err)
	}
}
