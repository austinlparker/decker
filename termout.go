package decker

import (
	"io"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"
)

// The live deck writes its own frames instead of using Bubble Tea's
// renderer. Bubble Tea's renderer takes each frame as an escape-coded
// string, parses it back into cells, and then updates the terminal with
// every shortcut the terminal claims to support: scrolling regions,
// repeated and inserted characters, erases. At 682×171 that costs ~8ms a
// frame, and when a shortcut lands differently than the renderer expects,
// stripes come out shifted or the wrong color until the whole screen is
// redrawn.
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

const (
	// Unchanged cells between two changes are rewritten rather than skipped
	// with a cursor move, if there are at most this many of them.
	rewriteGap = 8
)

func newTermWriter(out io.Writer, bg RGB) *termWriter {
	w := &termWriter{
		out:      out,
		notify:   make(chan struct{}, 1),
		quit:     make(chan struct{}),
		finished: make(chan struct{}),
	}
	q := bg.q()
	// Alternate screen, cursor hidden, no autowrap (so writing the last
	// column never scrolls), and the deck's background as the terminal's,
	// so window padding matches the slides.
	io.WriteString(out, "\x1b[?1049h\x1b[?25l\x1b[?7l\x1b]11;#"+hex2(q[0])+hex2(q[1])+hex2(q[2])+"\x07\x1b[2J")
	go w.loop()
	return w
}

func hex2(v uint8) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[v>>4], digits[v&15]})
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
	io.WriteString(w.out, "\x1b[0m\x1b[?7h\x1b[?25h\x1b[?1049l\x1b]111\x07")
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

func (w *termWriter) write(g *Grid, title string, full bool) {
	prev := w.prev
	if prev == nil || prev.W != g.W || prev.H != g.H {
		full = true
	}
	b := append(w.buf[:0], "\x1b[?2026h"...)
	start := len(b)
	if title != w.lastTitle {
		b = append(b, "\x1b]2;"...)
		b = append(b, title...)
		b = append(b, '\a')
		w.lastTitle = title
	}
	if full {
		b = append(b, "\x1b[0m\x1b[2J"...)
	}
	var p pen
	b = append(b, "\x1b[0m"...)
	for y := 0; y < g.H; y++ {
		row := g.Cells[y*g.W : (y+1)*g.W]
		if full {
			b = moveTo(b, 0, y)
			b = p.run(b, row, 0, g.W-1)
			continue
		}
		old := prev.Cells[y*g.W : (y+1)*g.W]
		for x := 0; x < g.W; {
			if row[x] == old[x] {
				x++
				continue
			}
			first := x
			if row[first].ch == "" && first > 0 {
				first-- // start at the wide character this half belongs to
			}
			last, gap := x, 0
			for x2 := x + 1; x2 < g.W && gap <= rewriteGap; x2++ {
				if row[x2] != old[x2] {
					last, gap = x2, 0
				} else {
					gap++
				}
			}
			b = moveTo(b, first, y)
			b = p.run(b, row, first, last)
			x = last + 1
		}
	}
	if len(b) > start+len("\x1b[0m") || full {
		b = append(b, "\x1b[?2026l"...)
		if _, err := w.out.Write(b); err != nil {
			// The terminal went away; nothing useful to do.
			_ = err
		}
	}
	w.buf = b
	w.prev.release()
	w.prev = g
}

// run appends cells first..last of row.
func (p *pen) run(b []byte, row []gcell, first, last int) []byte {
	for x := first; x <= last && x < len(row); x++ {
		c := &row[x]
		if c.ch == "" {
			continue
		}
		b = p.cell(b, c)
		if c.wide {
			x++
		}
	}
	return b
}

// moveTo appends a cursor move to column x, row y (0-based).
func moveTo(b []byte, x, y int) []byte {
	b = append(b, "\x1b["...)
	b = strconv.AppendInt(b, int64(y+1), 10)
	b = append(b, ';')
	b = strconv.AppendInt(b, int64(x+1), 10)
	return append(b, 'H')
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
