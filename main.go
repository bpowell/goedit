package main

import (
	"bytes"
	"os"
	"syscall"
	"unsafe"
)

type terminal int

func (t terminal) Read(buf []byte) (int, error) {
	return syscall.Read(int(t), buf)
}

func (t terminal) Write(s string) {
	b := bytes.NewBufferString(s)
	if _, err := syscall.Write(int(t), b.Bytes()); err != nil {
		panic(err)
	}
}

var orignial syscall.Termios
var reader terminal

func init() {
	reader = terminal(syscall.Stdin)
	_, _, err := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(reader), syscall.TCGETS, uintptr(unsafe.Pointer(&orignial)), 0, 0, 0)
	if err != 0 {
		panic(err)
	}
}

func rawMode() {
	argp := orignial
	argp.Iflag &^= syscall.BRKINT | syscall.ICRNL | syscall.INPCK | syscall.ISTRIP | syscall.IXON
	argp.Oflag &^= syscall.OPOST
	argp.Cflag |= syscall.CS8
	argp.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.ISIG
	argp.Cc[syscall.VMIN] = 0
	argp.Cc[syscall.VTIME] = 1

	_, _, err := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(reader), 0x5404, uintptr(unsafe.Pointer(&argp)), 0, 0, 0)
	if err != 0 {
		panic(err)
	}
}

func resetMode() {
	_, _, err := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(reader), 0x5404, uintptr(unsafe.Pointer(&orignial)), 0, 0, 0)
	if err != 0 {
		panic(err)
	}
}

func readKey() byte {
	var buf [1]byte

	for {
		n, err := reader.Read(buf[:])
		if err != nil {
			panic(err)
		}

		if n == 1 {
			break
		}
	}

	return buf[0]
}

func clearScreen() {
	reader.Write("\x1b[2J")
	reader.Write("\x1b[H")
}

func processKeyPress() {
	key := readKey()

	switch key {
	case ('q' & 0x1f):
		resetMode()
		os.Exit(0)
	}
}

func main() {
	rawMode()

	for {
		clearScreen()
		processKeyPress()
	}
}
