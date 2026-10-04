package decker

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// vterm is just enough of a terminal to replay what termWriter writes:
// cursor moves, 24-bit colors, attribute resets, clears, and text, with
// autowrap off.
type vterm struct {
	w, h   int
	cells  []gcell
	cx, cy int
	fg, bg [3]uint8
	attrs  uint8
}

func newVterm(w, h int) *vterm { return &vterm{w: w, h: h, cells: make([]gcell, w*h)} }

func (v *vterm) feed(t *testing.T, b []byte) {
	s := string(b)
	for i := 0; i < len(s); {
		switch {
		case strings.HasPrefix(s[i:], "\x1b]"): // OSC ... BEL
			end := strings.IndexByte(s[i:], '\a')
			if end < 0 {
				t.Fatalf("unterminated OSC")
			}
			i += end + 1
		case strings.HasPrefix(s[i:], "\x1b["):
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			v.csi(t, s[i+2:j], s[j])
			i = j + 1
		case s[i] == 0x1b:
			t.Fatalf("unexpected escape at %d: %q", i, s[i:min(i+10, len(s))])
		case s[i] < 0x20:
			t.Fatalf("unexpected control byte %#x", s[i])
		default:
			r, n := utf8.DecodeRuneInString(s[i:])
			ch := string(r)
			wide := r >= 0x1100 && (r < 0x2000 || r > 0x2bff)
			if v.cx < v.w {
				v.cells[v.cy*v.w+v.cx] = gcell{ch: ch, fg: v.fg, bg: v.bg, attrs: v.attrs, wide: wide}
				if wide && v.cx+1 < v.w {
					v.cells[v.cy*v.w+v.cx+1] = gcell{fg: v.fg, bg: v.bg}
				}
			}
			if wide {
				v.cx++
			}
			v.cx = min(v.cx+1, v.w) // no autowrap
			i += n
		}
	}
}

func (v *vterm) csi(t *testing.T, params string, final byte) {
	switch final {
	case 'H':
		p := strings.Split(params, ";")
		y, _ := strconv.Atoi(p[0])
		x, _ := strconv.Atoi(p[1])
		v.cx, v.cy = x-1, y-1
	case 'J':
		for i := range v.cells {
			v.cells[i] = gcell{}
		}
	case 'h', 'l': // modes
	case 'm':
		p := strings.Split(params, ";")
		for k := 0; k < len(p); k++ {
			switch p[k] {
			case "", "0":
				v.attrs = 0
			case "1":
				v.attrs |= 1
			case "38", "48":
				var c [3]uint8
				for n := 0; n < 3; n++ {
					x, _ := strconv.Atoi(p[k+2+n])
					c[n] = uint8(x)
				}
				if p[k] == "38" {
					v.fg = c
				} else {
					v.bg = c
				}
				k += 4
			default:
				v.attrs |= 0x80 // some other attribute; enough to notice
			}
		}
	default:
		t.Fatalf("unexpected CSI %q%c", params, final)
	}
}

// check compares the terminal with g. A plain space's foreground color
// isn't visible, so it isn't compared.
func (v *vterm) check(t *testing.T, g *Grid, what string) {
	t.Helper()
	for i, want := range g.Cells {
		got := v.cells[i]
		if want.ch == " " && want.attrs == 0 && want.ul == 0 {
			got.fg, want.fg = [3]uint8{}, [3]uint8{}
		}
		if got != want {
			t.Fatalf("%s: cell (%d,%d) = %+v, want %+v", what, i%g.W, i/g.W, got, want)
		}
	}
}

// TestTermWriter replays frames through termWriter and checks that the
// terminal ends up showing exactly each frame: the title slide animating,
// a slide transition, and a resize.
func TestTermWriter(t *testing.T) {
	d := testDeck()
	w, h := 120, 34
	var out bytes.Buffer
	tw := &termWriter{out: &out}
	vt := newVterm(w, h)

	m := newModel(d, 0, 0, 60, nil)
	m.w, m.h = w, h
	start := m.now
	for f := 0; f < 150; f++ {
		m.now = start.Add(time.Duration(f) * time.Second / 60)
		if f == 60 {
			m.goTo(1, 0, true) // a Push transition to slide 2
		}
		if m.transFrom != nil && m.now.Sub(m.transStart).Seconds() >= TransitionDuration {
			m.transFrom = nil
		}
		g := m.frame()
		out.Reset()
		tw.write(g.clone(), m.cur().Title, false)
		vt.feed(t, out.Bytes())
		vt.check(t, g, "frame "+strconv.Itoa(f))
		g.release()
	}

	// A frame that doesn't change writes nothing.
	g := m.frame()
	tw.write(g.clone(), m.cur().Title, false)
	out.Reset()
	tw.write(g, m.cur().Title, false)
	if out.Len() != 0 {
		t.Errorf("unchanged frame wrote %d bytes", out.Len())
	}

	// A resize redraws everything.
	w, h = 90, 30
	m.w, m.h = w, h
	vt = newVterm(w, h)
	g = m.frame()
	out.Reset()
	tw.write(g.clone(), m.cur().Title, false)
	vt.feed(t, out.Bytes())
	vt.check(t, g, "after resize")
}

// TestTermWriterWide checks wide characters survive partial updates.
func TestTermWriterWide(t *testing.T) {
	var out bytes.Buffer
	tw := &termWriter{out: &out}
	vt := newVterm(10, 1)
	a := parseGrid("ab界cd", 10, 1, testTheme)
	tw.write(a.clone(), "", false)
	vt.feed(t, out.Bytes())
	vt.check(t, a, "first")
	b := parseGrid("ab世cd", 10, 1, testTheme)
	out.Reset()
	tw.write(b.clone(), "", false)
	vt.feed(t, out.Bytes())
	vt.check(t, b, "second")
}
