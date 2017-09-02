package main

import (
	"fmt"
	"syscall"
	"unicode"
	"unicode/utf8"
	"unsafe"
)

type terminal int

func (t terminal) Read(buf []byte) (int, error) {
	return syscall.Read(int(t), buf)
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

func main() {
	defer resetMode()
	var buf [1]byte
	rawMode()

	for {
		if _, err := reader.Read(buf[:]); err != nil {
			panic(err)
		}

		r, _ := utf8.DecodeRune(buf[:])
		if unicode.IsControl(r) {
			fmt.Printf("%d\r\n", buf[0])
		} else {
			fmt.Printf("%d %c\r\n", buf[0], buf[0])
		}

		if buf[0] == 'q' {
			break
		}
	}
}
