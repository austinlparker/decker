package decker

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
)

// termWriter draws the live deck, replacing Bubble Tea's renderer, which
// re-parses each frame string and then updates the terminal with every
// shortcut it claims to support (scrolling regions, repeated and inserted
// characters, erases). At 682×171 that costs ~8ms a frame, and when a
// shortcut lands differently than the renderer expects, stripes come out
// shifted or the wrong color until the whole screen is redrawn.
//
// termWriter takes each frame as a Grid, compares it with the last frame it
// wrote, and sends only the changed cells, using nothing but cursor moves,
// colors and text. Each frame is wrapped in synchronized output, so the
// terminal shows it whole or not at all. Writing happens on its own
// goroutine: if the terminal falls behind, frames are dropped rather than
// queued, and the deck keeps responding to keys.
type termWriter struct {
	out io.Writer

	mu       sync.Mutex
	pending  *Grid
	title    string
	full     bool // redraw every cell next frame
	notify   chan struct{}
	quit     chan struct{}
	finished chan struct{}

	// Owned by the writing goroutine.
	prev      *Grid
	lastTitle string
	buf       []byte
}

// rewriteGap is how many unchanged cells between two changes are rewritten
// rather than skipped with a cursor move.
const rewriteGap = 8

func newTermWriter(out io.Writer, bg RGB) *termWriter {
	w := &termWriter{
		out:      out,
		notify:   make(chan struct{}, 1),
		quit:     make(chan struct{}),
		finished: make(chan struct{}),
	}
	q := bg.q()
	// No autowrap, so writing the last column never scrolls, and the deck's
	// background as the terminal's, so window padding matches the slides.
	io.WriteString(out, ansi.SetModeAltScreenSaveCursor+ansi.HideCursor+ansi.ResetModeAutoWrap+
		ansi.SetBackgroundColor(fmt.Sprintf("#%02x%02x%02x", q[0], q[1], q[2]))+ansi.EraseEntireScreen)
	go w.loop()
	return w
}

// submit hands a frame to the writer, which owns it from now on. A frame
// that hasn't been written yet is dropped in favor of the new one.
func (w *termWriter) submit(g *Grid, title string) {
	w.mu.Lock()
	w.pending.release()
	w.pending, w.title = g, title
	w.mu.Unlock()
	select {
	case w.notify <- struct{}{}:
	default:
	}
}

// invalidate makes the next frame redraw every cell, in case something
// else drew on the terminal.
func (w *termWriter) invalidate() {
	w.mu.Lock()
	w.full = true
	w.mu.Unlock()
}

// close stops the writer and restores the terminal.
func (w *termWriter) close() {
	close(w.quit)
	<-w.finished
	io.WriteString(w.out, "\x1b[0m"+ansi.SetModeAutoWrap+ansi.ShowCursor+ansi.ResetModeAltScreenSaveCursor+ansi.ResetBackgroundColor)
}

func (w *termWriter) loop() {
	defer close(w.finished)
	for {
		select {
		case <-w.quit:
			return
		case <-w.notify:
		}
		w.mu.Lock()
		g, title, full := w.pending, w.title, w.full
		w.pending, w.full = nil, false
		w.mu.Unlock()
		if g == nil {
			continue
		}
		w.write(g, title, full)
	}
}

// write encodes g as a diff against the previous frame, or in full when full
// is set or the size changed, and takes ownership of g.
func (w *termWriter) write(g *Grid, title string, full bool) {
	prev := w.prev
	full = full || prev == nil || prev.W != g.W || prev.H != g.H
	b := append(w.buf[:0], ansi.SetModeSynchronizedOutput...)
	changed := full
	if title != w.lastTitle {
		b = append(b, ansi.SetWindowTitle(title)...)
		w.lastTitle, changed = title, true
	}
	if full {
		b = append(b, "\x1b[0m"+ansi.EraseEntireScreen...)
	}
	var p pen
	b = append(b, "\x1b[0m"...)
	mark := len(b)
	for y := range g.H {
		row := g.Cells[y*g.W : (y+1)*g.W]
		if full {
			b = p.run(moveTo(b, 0, y), row, 0, g.W-1)
		} else {
			b = diffRow(b, &p, row, prev.Cells[y*g.W:(y+1)*g.W], y)
		}
	}
	if changed || len(b) > mark {
		b = append(b, ansi.ResetModeSynchronizedOutput...)
		_, _ = w.out.Write(b) // if the terminal went away there's nothing useful to do
	}
	w.buf = b
	w.prev.release()
	w.prev = g
}

// diffRow appends the cursor moves and cells that turn old into row, which is
// row y. Changes at most rewriteGap cells apart are written as one run.
func diffRow(b []byte, p *pen, row, old []gcell, y int) []byte {
	for x := 0; x < len(row); {
		if row[x] == old[x] {
			x++
			continue
		}
		first := x
		if row[first].ch == "" && first > 0 {
			first-- // start at the wide character this half belongs to
		}
		last, gap := x, 0
		for x2 := x + 1; x2 < len(row) && gap <= rewriteGap; x2++ {
			if row[x2] != old[x2] {
				last, gap = x2, 0
			} else {
				gap++
			}
		}
		b = p.run(moveTo(b, first, y), row, first, last)
		x = last + 1
	}
	return b
}

// The terminal encoder appends bytes by hand: it runs for every changed cell
// of every frame, and x/ansi's helpers allocate.

// moveTo appends a cursor move to column x, row y (0-based).
func moveTo(b []byte, x, y int) []byte {
	b = append(b, "\x1b["...)
	b = strconv.AppendInt(b, int64(y+1), 10)
	b = append(b, ';')
	b = strconv.AppendInt(b, int64(x+1), 10)
	return append(b, 'H')
}

// String encodes the grid as exactly H lines of exactly W cells. Each cell is
// a half block (▀) whose foreground is the top pixel and background the
// bottom one, or a space when both match. Every cell gets an explicit
// background color: terminals with a translucent background only make the
// default background see-through, and the deck should be opaque. Colors are
// only written when they change, to keep frames small: at 682×171 a frame is
// 117k cells, and the terminal has to parse all of it.
func (g *Grid) String() string {
	var b strings.Builder
	b.Grow(g.W * g.H * 6)
	var buf []byte
	for y := range g.H {
		var p pen
		row := g.Cells[y*g.W : (y+1)*g.W]
		buf = p.run(buf[:0], row, 0, len(row)-1)
		b.Write(buf)
		if p.on {
			b.WriteString("\x1b[m")
		}
		if y < g.H-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// pen tracks the terminal's current colors and attributes, so encoding
// only writes what changes.
type pen struct {
	fg, bg     [3]uint8
	fgOn, bgOn bool
	attrs, ul  uint8
	on         bool // anything set since the last reset
}

// cell appends the escape codes and text for c to buf and returns it.
func (p *pen) cell(buf []byte, c *gcell) []byte {
	if c.attrs != p.attrs || c.ul != p.ul {
		// Attributes can only be turned off together: reset, then set.
		buf = append(buf, "\x1b[0"...)
		st := uv.Style{Attrs: c.attrs, Underline: uv.Underline(c.ul)}
		if c.attrs != 0 || c.ul != 0 {
			if s := st.String(); len(s) > 3 { // "\x1b[" + params + "m"
				buf = append(buf, ';')
				buf = append(buf, s[2:len(s)-1]...)
			}
		}
		buf = append(buf, 'm')
		p.fgOn, p.bgOn = false, false
		p.attrs, p.ul, p.on = c.attrs, c.ul, true
	}
	// A plain space shows only its background; skip changing the foreground.
	plainSpace := c.ch == " " && c.attrs == 0 && c.ul == 0
	if !plainSpace && (!p.fgOn || p.fg != c.fg) {
		buf = sgrColor(buf, '3', c.fg)
		p.fg, p.fgOn, p.on = c.fg, true, true
	}
	if !p.bgOn || p.bg != c.bg {
		buf = sgrColor(buf, '4', c.bg)
		p.bg, p.bgOn, p.on = c.bg, true, true
	}
	return append(buf, c.ch...)
}

// run appends cells first..last of row.
func (p *pen) run(b []byte, row []gcell, first, last int) []byte {
	for x := first; x <= last && x < len(row); x++ {
		c := &row[x]
		if c.ch == "" {
			continue // drawn by the wide character before it
		}
		b = p.cell(b, c)
		if c.wide {
			x++
		}
	}
	return b
}

// sgrColor appends a 24-bit color: kind '3' for foreground, '4' background.
func sgrColor(buf []byte, kind byte, c [3]uint8) []byte {
	buf = append(buf, "\x1b["...)
	buf = append(buf, kind, '8', ';', '2', ';')
	buf = strconv.AppendUint(buf, uint64(c[0]), 10)
	buf = append(buf, ';')
	buf = strconv.AppendUint(buf, uint64(c[1]), 10)
	buf = append(buf, ';')
	buf = strconv.AppendUint(buf, uint64(c[2]), 10)
	return append(buf, 'm')
}

// liveTerminal sets up the terminal for the live deck: raw keyboard input,
// window size reports and the frame writer. Bubble Tea runs without its
// renderer, which also means it leaves the keyboard and resizing to us.
type liveTerminal struct {
	writer  *termWriter
	restore func()
	sigs    chan os.Signal
}

func startLiveTerminal(t *Theme) (*liveTerminal, error) {
	lt := &liveTerminal{restore: func() {}}
	if term.IsTerminal(os.Stdin.Fd()) {
		state, err := term.MakeRaw(os.Stdin.Fd())
		if err != nil {
			return nil, err
		}
		lt.restore = func() { _ = term.Restore(os.Stdin.Fd(), state) }
	}
	lt.writer = newTermWriter(os.Stdout, t.Background)
	return lt, nil
}

// watchSize sends the window size to p now and whenever it changes.
func (lt *liveTerminal) watchSize(p *tea.Program) {
	send := func() {
		if w, h, err := term.GetSize(os.Stdout.Fd()); err == nil {
			p.Send(tea.WindowSizeMsg{Width: w, Height: h})
		}
	}
	lt.sigs = make(chan os.Signal, 1)
	signal.Notify(lt.sigs, syscall.SIGWINCH)
	go func() {
		send()
		for range lt.sigs {
			send()
		}
	}()
}

func (lt *liveTerminal) close() {
	if lt.sigs != nil {
		signal.Stop(lt.sigs)
		close(lt.sigs)
	}
	lt.writer.close()
	lt.restore()
}
