package main

import (
	"bytes"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

const (
	CURSOR_UP    = 1000
	CURSOR_DOWN  = 1001
	CURSOR_LEFT  = 1002
	CURSOR_RIGHT = 1003
)

const (
	INSERT_MODE = 1
	CMD_MODE    = 2
)

type winsize struct {
	height uint16
	width  uint16
	x      uint16
	y      uint16
}

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

type cursor struct {
	x, y uint16
}

type editor struct {
	reader   terminal
	orignial syscall.Termios
	winsize
	contents *bytes.Buffer
	cursor   cursor
	mode     int
}

var goedit editor

func init() {
	goedit = editor{}
	goedit.mode = CMD_MODE

	goedit.reader = terminal(syscall.Stdin)
	_, _, err := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(goedit.reader), syscall.TCGETS, uintptr(unsafe.Pointer(&goedit.orignial)), 0, 0, 0)
	if err != 0 {
		panic(err)
	}

	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, uintptr(goedit.reader), syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&goedit.winsize))); err != 0 {
		panic(err)
	}

	goedit.contents = bytes.NewBufferString("")
}

func drawRows() {
	for x := 0; x < int(goedit.height); x++ {
		goedit.contents.WriteString("~")
		goedit.contents.WriteString("\x1b[K")
		if x < int(goedit.height)-1 {
			goedit.contents.WriteString("\r\n")
		}
	}
}

func rawMode() {
	argp := goedit.orignial
	argp.Iflag &^= syscall.BRKINT | syscall.ICRNL | syscall.INPCK | syscall.ISTRIP | syscall.IXON
	argp.Oflag &^= syscall.OPOST
	argp.Cflag |= syscall.CS8
	argp.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.ISIG
	argp.Cc[syscall.VMIN] = 0
	argp.Cc[syscall.VTIME] = 1

	_, _, err := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(goedit.reader), 0x5404, uintptr(unsafe.Pointer(&argp)), 0, 0, 0)
	if err != 0 {
		panic(err)
	}
}

func resetMode() {
	_, _, err := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(goedit.reader), 0x5404, uintptr(unsafe.Pointer(&goedit.orignial)), 0, 0, 0)
	if err != 0 {
		panic(err)
	}
}

func readKey() rune {
	var buf [1]byte

	for {
		n, err := goedit.reader.Read(buf[:])
		if err != nil {
			panic(err)
		}

		if n == 1 {
			break
		}
	}

	if buf[0] == '\x1b' {
		var seq [2]byte
		n, err := goedit.reader.Read(seq[:])
		if err != nil {
			panic(err)
		}

		if n != 2 {
			return '\x1b'
		}

		if seq[0] == '[' {
			switch seq[1] {
			case 'A':
				return CURSOR_UP
			case 'B':
				return CURSOR_DOWN
			case 'C':
				return CURSOR_RIGHT
			case 'D':
				return CURSOR_LEFT
			}
		}

		return '\x1b'
	}

	return bytes.Runes(buf[:])[0]
}

func (e *editor) moveCursor(key rune) {
	switch key {
	case CURSOR_DOWN:
		if e.height != e.cursor.y {
			e.cursor.y++
		}
	case CURSOR_UP:
		if e.cursor.y != 0 {
			e.cursor.y--
		}
	case CURSOR_LEFT:
		if e.cursor.x != 0 {
			e.cursor.x--
		}
	case CURSOR_RIGHT:
		if e.width != e.cursor.x {
			e.cursor.x++
		}
	}
}

func clearScreen() {
	goedit.contents.Reset()
	goedit.contents.WriteString("\x1b[?25l")
	goedit.contents.WriteString("\x1b[H")
	drawRows()
	goedit.contents.WriteString(fmt.Sprintf("\x1b[%d;%dH", int(goedit.cursor.y)+1, int(goedit.cursor.x)+1))
	goedit.contents.WriteString("\x1b[?25h")

	goedit.reader.Write(goedit.contents.String())
	goedit.contents.Reset()
}

func processKeyPress() {
	key := readKey()

	switch key {
	case ('q' & 0x1f):
		resetMode()
		os.Exit(0)
	case CURSOR_DOWN, CURSOR_UP, CURSOR_LEFT, CURSOR_RIGHT:
		goedit.moveCursor(key)
	case 'h':
		if goedit.mode == CMD_MODE {
			goedit.moveCursor(CURSOR_LEFT)
		}
	case 'j':
		if goedit.mode == CMD_MODE {
			goedit.moveCursor(CURSOR_DOWN)
		}
	case 'k':
		if goedit.mode == CMD_MODE {
			goedit.moveCursor(CURSOR_UP)
		}
	case 'l':
		if goedit.mode == CMD_MODE {
			goedit.moveCursor(CURSOR_RIGHT)
		}
	case 'i':
		if goedit.mode == CMD_MODE {
			goedit.mode = INSERT_MODE
		}
	case '\x1b':
		goedit.mode = CMD_MODE
	}
}

func main() {
	rawMode()

	for {
		clearScreen()
		processKeyPress()
	}
}
