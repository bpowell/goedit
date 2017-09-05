package main

import (
	"bufio"
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
	PAGE_UP      = 1004
	PAGE_DOWN    = 1005
	HOME_KEY     = 1006
	END_KEY      = 1007
	DEL_KEY      = 1008
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
	editorUI     *bytes.Buffer
	cursor       cursor
	mode         int
	fileContents []string
	filename     string
	fileOffSet   uint16
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

	goedit.editorUI = bytes.NewBufferString("")
}

func openFile(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		goedit.fileContents = append(goedit.fileContents, scanner.Text())
	}
}

func drawRows() {
	for x := 0; x < int(goedit.height); x++ {
		filerow := x + int(goedit.fileOffSet)
		if filerow >= len(goedit.fileContents) {
			goedit.editorUI.WriteString("~")
		} else {
			goedit.editorUI.WriteString(goedit.fileContents[filerow])
		}

		goedit.editorUI.WriteString("\x1b[K")
		if x < int(goedit.height)-1 {
			goedit.editorUI.WriteString("\r\n")
		}
	}
}

func scroll() {
	if goedit.cursor.y < goedit.fileOffSet {
		goedit.fileOffSet = goedit.cursor.y
	}

	if goedit.cursor.y >= goedit.fileOffSet+uint16(len(goedit.fileContents)) {
		goedit.fileOffSet = goedit.cursor.y - uint16(len(goedit.fileContents)) + 1
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
			if seq[1] >= '0' && seq[1] <= '9' {
				var tilde [1]byte
				n, err := goedit.reader.Read(tilde[:])
				if err != nil {
					panic(err)
				}

				if n != 1 {
					return '\x1b'
				}

				if tilde[0] == '~' {
					switch seq[1] {
					case '1', '7':
						return HOME_KEY
					case '3':
						return DEL_KEY
					case '4', '8':
						return END_KEY
					case '5':
						return PAGE_UP
					case '6':
						return PAGE_DOWN
					}
				}
			} else {
				switch seq[1] {
				case 'A':
					return CURSOR_UP
				case 'B':
					return CURSOR_DOWN
				case 'C':
					return CURSOR_RIGHT
				case 'D':
					return CURSOR_LEFT
				case 'H':
					return HOME_KEY
				case 'F':
					return END_KEY
				}
			}
		} else if seq[0] == 'O' {
			switch seq[1] {
			case 'H':
				return HOME_KEY
			case 'F':
				return END_KEY
			}
		}

		return '\x1b'
	}

	return bytes.Runes(buf[:])[0]
}

func (e *editor) moveCursor(key rune) {
	switch key {
	case CURSOR_DOWN:
		if e.cursor.y < uint16(len(e.fileContents)) {
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
		if e.width != e.cursor.x-uint16(1) {
			e.cursor.x++
		}
	}
}

func clearScreen() {
	scroll()
	goedit.editorUI.Reset()
	goedit.editorUI.WriteString("\x1b[?25l")
	goedit.editorUI.WriteString("\x1b[H")
	drawRows()
	goedit.editorUI.WriteString(fmt.Sprintf("\x1b[%d;%dH", int(goedit.cursor.y-goedit.fileOffSet)+1, int(goedit.cursor.x)+1))
	goedit.editorUI.WriteString("\x1b[?25h")

	goedit.reader.Write(goedit.editorUI.String())
	goedit.editorUI.Reset()
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
	case PAGE_UP:
		for x := 0; x < int(goedit.height); x++ {
			goedit.moveCursor(CURSOR_UP)
		}
	case PAGE_DOWN:
		for x := 0; x < int(goedit.height); x++ {
			goedit.moveCursor(CURSOR_DOWN)
		}
	case HOME_KEY:
		goedit.cursor.x = 0
	case END_KEY:
		goedit.cursor.x = goedit.width - 1
	}
}

func main() {
	rawMode()
	if len(os.Args) == 2 {
		openFile(os.Args[1])
	}

	for {
		clearScreen()
		processKeyPress()
	}
}
