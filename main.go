package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode"
	"unsafe"
)

var errorlog *os.File
var logger *log.Logger

const (
	TAB_STOP = 4
)

const (
	BACKSPACE    = 127
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
	NORMAL_MODE = 2
	CMD_MODE    = 3
)

const (
	BLACK   = 30
	RED     = 31
	GREEN   = 32
	YELLOW  = 33
	BLUE    = 34
	MAGENTA = 35
	CYAN    = 36
	WHITE   = 37
)

const (
	HL_COMMENTS   = RED
	HL_STATEMENTS = YELLOW
	HL_TYPES      = GREEN
)

const (
	FORWARD  = 1
	BACKWARD = 2
)

type hl_groups struct {
	Comments   []string `json:"comments"`
	Statements []string `josn:"statements"`
	Types      []string `json:"types"`
}

type winsize struct {
	height uint16
	width  uint16
	x      uint16
	y      uint16
}

type erow struct {
	chars     string
	render    string
	size      int
	rsize     int
	highlight []int
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

type searchObject struct {
	query    string
	location cursor
}

type editorMsgBar struct {
	msg     string
	bgColor int
	fgColor int
}

type editor struct {
	reader        terminal
	orignial      syscall.Termios
	height        int
	width         int
	editorUI      *bytes.Buffer
	cursor        cursor
	mode          int
	filename      string
	rowOffSet     int
	colOffSet     int
	numOfRows     int
	rows          []erow
	rx            int
	editormsg     editorMsgBar
	lineNumOffSet int
	search        searchObject
	modifiyed     bool
}

func (r *erow) updateRow() {
	r.highlight = nil
	buf := bytes.NewBufferString("")
	raw := []byte(r.chars)
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
	r.rsize = rsize
	r.render = buf.String()
	r.updateSyntax()
}

func (r *erow) updateSyntax() {
	raw := []byte(r.render)
	for x := 0; x < r.rsize; x++ {
		if unicode.IsDigit(rune(raw[x])) && x != 0 && !unicode.IsLetter(rune(raw[x-1])) {
			r.highlight = append(r.highlight, MAGENTA)
		} else {
			r.highlight = append(r.highlight, WHITE)
		}
	}
}

func (r *erow) deleteRune(pos int) {
	if pos < 0 || pos > r.size {
		return
	}

	buf := bytes.NewBufferString("")
	raw := []byte(r.chars)

	if pos == 0 {
		buf.Write(raw[1:])
	} else if pos == r.size {
		buf.Write(raw[0 : len(raw)-2])
	} else {
		buf.Write(raw[0:pos])
		buf.Write(raw[pos+1:])
	}

	r.chars = buf.String()
	r.size--
	r.updateRow()
}

func editorReplaceRune() {
	key := readKey()
	if key < 32 || key > 126 {
		return
	}

	goedit.moveCursor(CURSOR_RIGHT)
	editorDelRune()
	editorInsertRune(key)
	goedit.modifiyed = true
}

func editorDelFromCursorToEndOfLine() {
	raw := []byte(goedit.rows[goedit.cursor.y].chars)
	buf := bytes.NewBuffer(raw[:goedit.cursor.x])
	buf.WriteByte('\000')

	goedit.rows[goedit.cursor.y].chars = buf.String()
	goedit.rows[goedit.cursor.y].size = len(buf.String())
	goedit.rows[goedit.cursor.y].updateRow()
	goedit.modifiyed = true
}

func editorDelRune() {
	if goedit.cursor.y == goedit.numOfRows {
		return
	}

	if goedit.cursor.x == 0 && goedit.cursor.y == 0 {
		return
	}

	if goedit.cursor.x > 0 {
		goedit.rows[goedit.cursor.y].deleteRune(goedit.cursor.x - 1)
		goedit.cursor.x--
	} else {
		goedit.cursor.x = goedit.rows[goedit.cursor.y-1].size
		goedit.rows[goedit.cursor.y-1].appendRow(goedit.rows[goedit.cursor.y].chars)
		editorDelRow(goedit.cursor.y)
		goedit.cursor.y--
	}
	goedit.modifiyed = true
}

func editorDelRow(pos int) {
	if pos < 0 || pos >= goedit.numOfRows {
		return
	}

	goedit.rows = append(goedit.rows[:pos], goedit.rows[pos+1:]...)
	goedit.numOfRows--
	goedit.modifiyed = true
}

func (r *erow) appendRow(chars string) {
	r.chars = fmt.Sprintf("%s%s\000", r.chars, chars)
	r.size = len(r.chars)
	r.updateRow()
}

func (r *erow) insertRune(c rune, pos int) {
	if pos < 0 || pos > r.size {
		pos = r.size
	}

	buf := bytes.NewBufferString("")
	raw := []byte(r.chars)

	if pos == 0 {
		buf.WriteRune(c)
		buf.WriteString(r.chars)
	} else if pos == r.size {
		buf.WriteString(r.chars)
		buf.WriteRune(c)
	} else {
		buf.WriteString(string(raw[0:pos]))
		buf.WriteRune(c)
		buf.WriteString(string(raw[pos:]))
	}

	r.chars = buf.String()
	r.size = len(r.chars)
	r.updateRow()
}

func editorInsertRune(c rune) {
	if goedit.cursor.y == goedit.numOfRows {
		goedit.insertRow(goedit.numOfRows, "")
	}

	goedit.rows[goedit.cursor.y].insertRune(c, goedit.cursor.x)
	goedit.cursor.x++
	goedit.modifiyed = true
}

func (e *editor) insertRow(pos int, r string) {
	if pos < 0 || pos > e.numOfRows {
		return
	}

	buf := bytes.NewBufferString(strings.TrimRight(r, "\000"))
	buf.WriteByte('\000')
	row := erow{chars: buf.String()}
	row.size = len(row.chars) - 1

	row.updateRow()

	if pos == 0 {
		var rows []erow
		rows = append(rows, row)
		goedit.rows = append(rows, goedit.rows...)
	} else if pos == goedit.numOfRows {
		goedit.rows = append(goedit.rows, row)
	} else {
		rows := append(goedit.rows[:pos-1], row)
		goedit.rows = append(rows, goedit.rows[pos-1:]...)
	}

	goedit.numOfRows++
	e.lineNumOffSet = int(math.Log10(float64(e.numOfRows))) + 2
}

func editorInsertNewline() {
	if goedit.cursor.x == 0 {
		goedit.insertRow(goedit.cursor.y, "")
	} else {
		raw := []byte(goedit.rows[goedit.cursor.y].chars)
		newline := string(raw[goedit.cursor.x:])
		oldline := fmt.Sprintf("%s\000", string(raw[:goedit.cursor.x]))
		goedit.insertRow(goedit.cursor.y+1, newline)
		goedit.rows[goedit.cursor.y].chars = oldline
		goedit.rows[goedit.cursor.y].size = len(oldline)
		goedit.rows[goedit.cursor.y].updateRow()
	}

	goedit.cursor.x = 0
	goedit.cursor.y++
	goedit.modifiyed = true
}

func editorPrompt(msg string) string {
	oldcursor := goedit.cursor
	buf := bytes.NewBufferString("")
	msgLength := len(msg)
	goedit.cursor.x = msgLength
	goedit.editormsg.fgColor = WHITE
	goedit.editormsg.bgColor = 49

	for {
		goedit.editormsg.msg = fmt.Sprintf("%s%s", msg, buf)
		goedit.cursor.y = goedit.height + 1
		clearScreen()

		key := readKey()
		switch key {
		case '\r':
			//goedit.editormsg = ""
			goedit.cursor = oldcursor
			return buf.String()
		case BACKSPACE:
			if goedit.cursor.x <= msgLength {
				break
			}

			if goedit.cursor.x == msgLength {
				buf.Reset()
			} else if goedit.cursor.x == len(goedit.editormsg.msg) {
				tmp := bytes.NewBuffer(buf.Bytes()[:len(buf.String())-1])
				buf = tmp
			} else {
				tmp := bytes.NewBuffer(buf.Bytes()[:goedit.cursor.x-2])
				tmp.Write(buf.Bytes()[goedit.cursor.x-1:])
				buf = tmp
			}

			goedit.cursor.x--
		case CURSOR_LEFT:
			if goedit.cursor.x != msgLength {
				goedit.cursor.x--
			}
		case CURSOR_RIGHT:
			if goedit.cursor.x < len(goedit.editormsg.msg) {
				goedit.cursor.x++
			}
		default:
			buf.WriteRune(key)
			goedit.cursor.x++
		}
	}
}

func cursorxToRx(row erow, cx int) int {
	rx := 0
	raw := []byte(row.chars)
	for x := 0; x < cx; x++ {
		if raw[x] == '\t' {
			rx += (TAB_STOP - 1) - (rx % TAB_STOP)
		}
		rx++
	}

	return rx
}

func cursorxToCx(row erow, rx int) int {
	cur_rx := 0
	cx := 0
	for cx = 0; cx < row.size; cx++ {
		if row.chars[cx] == '\t' {
			cur_rx += (TAB_STOP - 1) - (cur_rx % TAB_STOP)
		}
		cur_rx++

		if cur_rx > rx {
			return cx
		}
	}

	return cx
}

var goedit editor

func init() {
	errorlog, errr := os.OpenFile("log.txt", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if errr != nil {
		logger.Fatal(errr)
	}

	logger = log.New(errorlog, "goedit: ", log.Lshortfile|log.LstdFlags)

	goedit = editor{}
	goedit.mode = NORMAL_MODE

	goedit.reader = terminal(syscall.Stdin)
	_, _, err := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(goedit.reader), syscall.TCGETS, uintptr(unsafe.Pointer(&goedit.orignial)), 0, 0, 0)
	if err != 0 {
		logger.Fatal(err)
	}

	winsize := winsize{}
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, uintptr(goedit.reader), syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&winsize))); err != 0 {
		logger.Fatal(err)
	}
	goedit.height = int(winsize.height) - 2
	goedit.width = int(winsize.width)

	goedit.editorUI = bytes.NewBufferString("")
	goedit.editormsg.fgColor = WHITE
	goedit.editormsg.bgColor = 49
}

func openFile(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		logger.Fatal(err)
	}
	defer file.Close()

	line := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		goedit.insertRow(line, scanner.Text())
		line++
	}

	goedit.numOfRows = len(goedit.rows)
	goedit.filename = filename
	selectSyntax(filename)
}

func selectSyntax(filename string) {
	var syntaxFile string
	switch filepath.Ext(filename) {
	case ".go":
		syntaxFile = "go.json"
	default:
		return
	}

	syntax, errr := os.Open(syntaxFile)
	if errr != nil {
		logger.Println("no syntax file found")
		return
	}
	defer syntax.Close()

	var syn hl_groups
	decoder := json.NewDecoder(syntax)
	if errr := decoder.Decode(&syn); errr != nil {
		logger.Println(errr)
		return
	}
}

func drawStatusBar() {
	status := fmt.Sprintf("%.20s - %d lines", goedit.filename, goedit.numOfRows)
	length := len(status)
	rstatus := fmt.Sprintf("%d,%d", goedit.cursor.y+1, goedit.rx+1)
	rlength := len(rstatus)
	goedit.editorUI.WriteString("\x1b[7m")
	goedit.editorUI.WriteString(status)
	for x := length; x < goedit.width; x++ {
		if goedit.width-x == rlength {
			goedit.editorUI.WriteString(rstatus)
			break
		} else {
			goedit.editorUI.WriteString(" ")
		}
	}

	goedit.editorUI.WriteString("\x1b[m")
	goedit.editorUI.WriteString("\r\n")
}

func drawMessageBar() {
	goedit.editorUI.WriteString("\x1b[K")
	goedit.editorUI.WriteString(fmt.Sprintf("\x1b[%d;%dm", goedit.editormsg.fgColor, goedit.editormsg.bgColor))
	goedit.editorUI.WriteString(goedit.editormsg.msg)
	goedit.editorUI.WriteString("\x1b[39;49m")
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
			formatter := fmt.Sprintf("%%%dd ", goedit.lineNumOffSet-1)
			goedit.editorUI.WriteString(fmt.Sprintf("\x1b[%dm", GREEN))
			goedit.editorUI.WriteString(fmt.Sprintf(formatter, filerow+1))
			goedit.editorUI.WriteString("\x1b[39;49m")
			for i := goedit.colOffSet; i < length; i++ {
				goedit.editorUI.WriteString(fmt.Sprintf("\x1b[%dm", goedit.rows[filerow].highlight[i]))
				goedit.editorUI.WriteByte(text[i])
			}
		}

		goedit.editorUI.WriteString("\x1b[K")
		goedit.editorUI.WriteString("\r\n")
	}
}

func scroll() {
	if goedit.mode != CMD_MODE {
		goedit.rx = 0
		if goedit.cursor.y < goedit.numOfRows {
			goedit.rx = cursorxToRx(goedit.rows[goedit.cursor.y], goedit.cursor.x)
		}

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

func (e *editor) rowsToString() string {
	buf := bytes.NewBufferString("")
	for _, r := range e.rows {
		raw := []byte(r.chars)
		raw[len(raw)-1] = '\n'
		buf.Write(raw)
	}

	return buf.String()
}

func (e *editor) save() {
	if e.filename == "" {
		e.filename = editorPrompt("Save as ")
		selectSyntax(e.filename)
	}

	file, err := os.OpenFile(e.filename, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		logger.Fatal(err)
	}
	defer file.Close()

	text := e.rowsToString()
	length := len(text)

	if err := file.Truncate(int64(length)); err != nil {
		e.editormsg.msg = err.Error()
		return
	}

	n, err := file.WriteAt([]byte(text), 0)
	if err != nil {
		e.editormsg.msg = err.Error()
		return
	}

	e.editormsg.msg = fmt.Sprintf("\"%s\" %dL %d bytess written to disk", e.filename, e.numOfRows, n)
	goedit.modifiyed = false
}

func clearScreen() {
	scroll()
	goedit.editorUI.Reset()
	goedit.editorUI.WriteString("\x1b[?25l")
	goedit.editorUI.WriteString("\x1b[H")
	drawRows()
	drawStatusBar()
	drawMessageBar()
	if goedit.mode == CMD_MODE {
		goedit.editorUI.WriteString(fmt.Sprintf("\x1b[%d;%dH", goedit.cursor.y+1, goedit.cursor.x+1))
	} else {
		goedit.editorUI.WriteString(fmt.Sprintf("\x1b[%d;%dH", (goedit.cursor.y-goedit.rowOffSet)+1, (goedit.rx-goedit.colOffSet)+1+goedit.lineNumOffSet))
	}
	goedit.editorUI.WriteString("\x1b[?25h")

	goedit.reader.Write(goedit.editorUI.String())
	goedit.editorUI.Reset()
}

func editorSearch() {
	query := editorPrompt("/")
	for i, row := range goedit.rows {
		indx := strings.Index(row.render, query)
		if indx != -1 {
			goedit.cursor.y = i
			goedit.cursor.x = cursorxToCx(row, indx)
			goedit.search.location = goedit.cursor
			goedit.search.query = query
			return
		}
	}

	goedit.editormsg.msg = fmt.Sprintf("Pattern not found: %s", query)
	goedit.editormsg.fgColor = WHITE
	goedit.editormsg.bgColor = BLUE + 10
}

func editorNextSearch() {
	if goedit.search.query == "" {
		return
	}

	if goedit.search.location.x+1 < goedit.rows[goedit.search.location.y].rsize {
		raw := []byte(goedit.rows[goedit.search.location.y].render)
		indx := strings.Index(string(raw[goedit.search.location.x+1:]), goedit.search.query)
		if indx != -1 {
			goedit.cursor.x = cursorxToCx(goedit.rows[goedit.search.location.y], indx)
			goedit.search.location.x = indx
			return
		}
	}

	for i := goedit.search.location.y + 1; i < goedit.numOfRows; i++ {
		indx := strings.Index(goedit.rows[i].render, goedit.search.query)
		if indx != -1 {
			goedit.cursor.y = i
			goedit.cursor.x = cursorxToCx(goedit.rows[i], indx)
			goedit.search.location.y = i
			goedit.search.location.x = indx
			return
		}
	}

	goedit.editormsg.msg = "Hit bottom, starting from the top"
	goedit.editormsg.fgColor = RED
	goedit.search.location.x = 0
	goedit.search.location.y = 0
	editorNextSearch()
}

func editorPrevSearch() {
	if goedit.search.query == "" {
		return
	}

	if goedit.search.location.x-1 > 0 {
		raw := []byte(goedit.rows[goedit.search.location.y].render)
		indx := strings.Index(string(raw[:goedit.search.location.x-1]), goedit.search.query)
		if indx != -1 {
			goedit.cursor.x = cursorxToCx(goedit.rows[goedit.search.location.y], indx)
			goedit.search.location.x = indx
			return
		}
	}

	for i := goedit.search.location.y - 1; i > 0; i-- {
		indx := strings.Index(goedit.rows[i].render, goedit.search.query)
		if indx != -1 {
			goedit.cursor.y = i
			goedit.cursor.x = cursorxToCx(goedit.rows[i], indx)
			goedit.search.location.y = i
			goedit.search.location.x = indx
			return
		}
	}

	goedit.editormsg.msg = "Hit top, starting from the bottom"
	goedit.editormsg.fgColor = RED
	goedit.search.location.x = goedit.rows[goedit.numOfRows-1].rsize
	goedit.search.location.y = goedit.numOfRows - 1
	editorPrevSearch()
}

func editorQuit(force bool) {
	if goedit.modifiyed == true && force == false {
		goedit.editormsg.msg = "No write since last change (add ! to override)"
		goedit.editormsg.fgColor = WHITE
		goedit.editormsg.bgColor = BLUE + 10
		return
	}

	resetMode()
	os.Exit(0)
}

func editorFindInRow(direction int, modifier int) {
	key := readKey()
	if key < 32 || key > 126 {
		return
	}

	var text string
	if direction == FORWARD {
		text = string(goedit.rows[goedit.cursor.y].chars[goedit.cursor.x:])
	} else if direction == BACKWARD {
		text = string(goedit.rows[goedit.cursor.y].chars[:goedit.cursor.x])
	}

	indx := strings.IndexRune(text, key)
	if indx == -1 {
		return
	}

	goedit.cursor.x = indx + modifier
}

func editorCommandMode() {
	result := editorPrompt(":")
	cmd := strings.Split(result, " ")

	switch cmd[0] {
	case "q", "quit":
		editorQuit(false)
	case "q!", "quit!":
		editorQuit(true)
	case "w", "write":
		goedit.save()
	case "wq":
		goedit.save()
		resetMode()
		os.Exit(0)
	}
}

func processKeyPress() {
	key := readKey()

	if goedit.mode == NORMAL_MODE {
		switch key {
		case 'h':
			goedit.moveCursor(CURSOR_LEFT)
		case 'j':
			goedit.moveCursor(CURSOR_DOWN)
		case 'k':
			goedit.moveCursor(CURSOR_UP)
		case 'l':
			goedit.moveCursor(CURSOR_RIGHT)
		case ':':
			goedit.mode = CMD_MODE
			editorCommandMode()
			goedit.mode = NORMAL_MODE
		case '/':
			goedit.mode = CMD_MODE
			editorSearch()
			goedit.mode = NORMAL_MODE
		case 'i':
			goedit.mode = INSERT_MODE
			goedit.editormsg.msg = "-- INSERT --"
			return
		case 'n':
			editorNextSearch()
		case 'N':
			editorPrevSearch()
		case '$':
			key = END_KEY
		case '0':
			key = HOME_KEY
		case 'D':
			editorDelFromCursorToEndOfLine()
			goedit.moveCursor(CURSOR_LEFT)
		case 'C':
			editorDelFromCursorToEndOfLine()
			goedit.mode = INSERT_MODE
			goedit.editormsg.msg = "-- INSERT --"
			return
		case 'a':
			goedit.moveCursor(CURSOR_RIGHT)
			goedit.mode = INSERT_MODE
			goedit.editormsg.msg = "-- INSERT --"
			return
		case 'r':
			editorReplaceRune()
		case 'x':
			goedit.moveCursor(CURSOR_RIGHT)
			editorDelRune()
		case 't':
			editorFindInRow(FORWARD, -1)
		case 'T':
			editorFindInRow(BACKWARD, 1)
		case 'f':
			editorFindInRow(FORWARD, 0)
		case 'F':
			editorFindInRow(BACKWARD, 0)
		case 'O':
			goedit.insertRow(goedit.cursor.y, "")
			goedit.mode = INSERT_MODE
			goedit.editormsg.msg = "-- INSERT --"
			return
		}
	}

	switch key {
	case CURSOR_DOWN, CURSOR_UP, CURSOR_LEFT, CURSOR_RIGHT:
		goedit.moveCursor(key)
	case '\x1b':
		goedit.mode = NORMAL_MODE
		goedit.editormsg.msg = ""
		goedit.editormsg.fgColor = WHITE
		goedit.editormsg.bgColor = 49
	case PAGE_UP:
		goedit.cursor.y = goedit.rowOffSet
		for x := 0; x < goedit.height; x++ {
			goedit.moveCursor(CURSOR_UP)
		}
	case PAGE_DOWN:
		goedit.cursor.y = goedit.rowOffSet + goedit.height - 1
		if goedit.cursor.y > goedit.numOfRows {
			goedit.cursor.y = goedit.numOfRows
		}
		for x := 0; x < goedit.height; x++ {
			goedit.moveCursor(CURSOR_DOWN)
		}
	case HOME_KEY:
		goedit.cursor.x = 0
	case END_KEY:
		if goedit.cursor.y < goedit.numOfRows {
			goedit.cursor.x = goedit.rows[goedit.cursor.y].size
		}
	case BACKSPACE:
		if goedit.mode == NORMAL_MODE {
			return
		}
		editorDelRune()
	case DEL_KEY:
		if key == DEL_KEY {
			goedit.moveCursor(CURSOR_RIGHT)
		}
		editorDelRune()
	case '\r':
		if goedit.mode == INSERT_MODE {
			editorInsertNewline()
		}
	default:
		if goedit.mode == INSERT_MODE {
			editorInsertRune(key)
		}
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
