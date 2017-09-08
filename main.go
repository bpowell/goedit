package main

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os"
	"syscall"
	"unsafe"
)

var errorlog *os.File
var logger *log.Logger

const (
	TAB_STOP = 4
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

type erow struct {
	chars  string
	render string
	size   int
	rsize  int
}

type terminal int

func (t terminal) Read(buf []byte) (int, error) {
	return syscall.Read(int(t), buf)
}

func (t terminal) Write(s string) {
	b := bytes.NewBufferString(s)
	if _, err := syscall.Write(int(t), b.Bytes()); err != nil {
		logger.Fatal(err)
	}
}

type cursor struct {
	x, y int
}

type editor struct {
	reader    terminal
	orignial  syscall.Termios
	height    int
	width     int
	editorUI  *bytes.Buffer
	cursor    cursor
	mode      int
	filename  string
	rowOffSet int
	colOffSet int
	numOfRows int
	rows      []erow
	rx        int
}

func (e *editor) appendRow(r string) {
	buf := bytes.NewBufferString(r)
	buf.WriteByte('\000')
	row := erow{chars: buf.String()}
	row.size = len(row.chars)

	buf.Reset()
	raw := []byte(r)
	rsize := 0
	for x := 0; x < len(raw); x++ {
		if raw[x] == '\t' {
			for i := 0; i < TAB_STOP; i++ {
				rsize++
				buf.WriteByte(' ')
			}
		} else {
			rsize++
			buf.WriteByte(raw[x])
		}
	}
	buf.WriteByte('\000')
	row.rsize = rsize + 1
	row.render = buf.String()

	e.rows = append(e.rows, row)
}

var goedit editor

func init() {
	errorlog, errr := os.OpenFile("log.txt", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if errr != nil {
		logger.Fatal(errr)
	}

	logger = log.New(errorlog, "goedit: ", log.Lshortfile|log.LstdFlags)

	goedit = editor{}
	goedit.mode = CMD_MODE

	goedit.reader = terminal(syscall.Stdin)
	_, _, err := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(goedit.reader), syscall.TCGETS, uintptr(unsafe.Pointer(&goedit.orignial)), 0, 0, 0)
	if err != 0 {
		logger.Fatal(err)
	}

	winsize := winsize{}
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, uintptr(goedit.reader), syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&winsize))); err != 0 {
		logger.Fatal(err)
	}
	goedit.height = int(winsize.height)
	goedit.width = int(winsize.width)

	goedit.editorUI = bytes.NewBufferString("")
}

func openFile(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		logger.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		goedit.appendRow(scanner.Text())
	}

	goedit.numOfRows = len(goedit.rows)
}

func drawRows() {
	for x := 0; x < goedit.height; x++ {
		filerow := x + goedit.rowOffSet
		if filerow >= goedit.numOfRows {
			goedit.editorUI.WriteString("~")
		} else {
			length := goedit.rows[filerow].rsize - goedit.colOffSet
			if length < 0 {
				length = 0
			}

			if length > goedit.width {
				length = goedit.width
			}

			text := []byte(goedit.rows[filerow].render)
			goedit.editorUI.Write(text[goedit.colOffSet:length])
		}

		goedit.editorUI.WriteString("\x1b[K")
		if x < goedit.height-1 {
			goedit.editorUI.WriteString("\r\n")
		}
	}
}

func scroll() {
	goedit.rx = goedit.cursor.x

	if goedit.cursor.y < goedit.rowOffSet {
		goedit.rowOffSet = goedit.cursor.y
	}

	if goedit.cursor.y >= goedit.rowOffSet+goedit.height {
		goedit.rowOffSet = goedit.cursor.y - goedit.height + 1
	}

	if goedit.rx < goedit.colOffSet {
		goedit.colOffSet = goedit.rx
	}

	if goedit.rx >= goedit.colOffSet+goedit.width {
		goedit.colOffSet = goedit.rx - goedit.width + 1
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
		logger.Fatal(err)
	}
}

func resetMode() {
	_, _, err := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(goedit.reader), 0x5404, uintptr(unsafe.Pointer(&goedit.orignial)), 0, 0, 0)
	if err != 0 {
		logger.Fatal(err)
	}
}

func readKey() rune {
	var buf [1]byte

	for {
		n, err := goedit.reader.Read(buf[:])
		if err != nil {
			logger.Fatal(err)
		}

		if n == 1 {
			break
		}
	}

	if buf[0] == '\x1b' {
		var seq [2]byte
		n, err := goedit.reader.Read(seq[:])
		if err != nil {
			logger.Fatal(err)
		}

		if n != 2 {
			return '\x1b'
		}

		if seq[0] == '[' {
			if seq[1] >= '0' && seq[1] <= '9' {
				var tilde [1]byte
				n, err := goedit.reader.Read(tilde[:])
				if err != nil {
					logger.Fatal(err)
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
		if e.cursor.y < goedit.numOfRows {
			e.cursor.y++
		}
	case CURSOR_UP:
		if e.cursor.y != 0 {
			e.cursor.y--
		}
	case CURSOR_LEFT:
		if e.cursor.x != 0 {
			e.cursor.x--
		} else if e.cursor.y > 0 {
			e.cursor.y--
			e.cursor.x = e.rows[e.cursor.y].size
		}
	case CURSOR_RIGHT:
		if e.cursor.y < e.numOfRows && e.cursor.x < e.rows[e.cursor.y].size {
			e.cursor.x++
		} else if e.cursor.y < e.numOfRows && e.cursor.x == e.rows[e.cursor.y].size {
			e.cursor.y++
			e.cursor.x = 0
		}
	}

	if e.cursor.y < e.numOfRows {
		if e.cursor.x > e.rows[e.cursor.y].size {
			e.cursor.x = e.rows[e.cursor.y].size
		}
	}
}

func clearScreen() {
	scroll()
	goedit.editorUI.Reset()
	goedit.editorUI.WriteString("\x1b[?25l")
	goedit.editorUI.WriteString("\x1b[H")
	drawRows()
	goedit.editorUI.WriteString(fmt.Sprintf("\x1b[%d;%dH", (goedit.cursor.y-goedit.rowOffSet)+1, (goedit.cursor.x-goedit.colOffSet)+1))
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
		for x := 0; x < goedit.height; x++ {
			goedit.moveCursor(CURSOR_UP)
		}
	case PAGE_DOWN:
		for x := 0; x < goedit.height; x++ {
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
