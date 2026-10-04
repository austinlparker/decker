package decker

import (
	"image/color"
	"strconv"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// Grid is one finished frame as terminal cells: what the live deck writes to
// the terminal (see termout.go), and what the string output is encoded
// from. Slides build it straight from their pixel canvas, so the deck never
// has to turn a frame into escape codes and parse it back.
type Grid struct {
	W, H  int
	Cells []gcell
}

// gcell is one terminal cell. Most cells are a space or a half block (▀)
// with explicit colors; rich cells (help, notes, the footer) can carry text
// attributes.
type gcell struct {
	ch     string // one grapheme; "" for the second half of a wide character
	fg, bg [3]uint8
	attrs  uint8 // uv.Attr* bits
	ul     uint8 // uv.Underline style
	wide   bool  // ch takes two cells
}

const halfBlock = "▀"

// Grids are big (a 682×171 frame is ~117k cells) and one is made every
// frame, so they're recycled.
var (
	gridPoolMu sync.Mutex
	gridPool   []*Grid
)

// newGrid returns a w×h grid. Its cells are not cleared: callers fill every
// cell.
func newGrid(w, h int) *Grid {
	w, h = max(w, 1), max(h, 1)
	gridPoolMu.Lock()
	for i, g := range gridPool {
		if g.W == w && g.H == h {
			gridPool = append(gridPool[:i], gridPool[i+1:]...)
			gridPoolMu.Unlock()
			return g
		}
	}
	gridPoolMu.Unlock()
	return &Grid{W: w, H: h, Cells: make([]gcell, w*h)}
}

// blankGrid returns a grid filled with bg.
func blankGrid(w, h int, bg RGB) *Grid {
	g := newGrid(w, h)
	q := bg.q()
	for i := range g.Cells {
		g.Cells[i] = gcell{ch: " ", fg: q, bg: q}
	}
	return g
}

// release returns g to the pool. Don't use it afterwards.
func (g *Grid) release() {
	if g == nil {
		return
	}
	gridPoolMu.Lock()
	if len(gridPool) < 6 {
		gridPool = append(gridPool, g)
	}
	gridPoolMu.Unlock()
}

func (g *Grid) at(x, y int) *gcell { return &g.Cells[y*g.W+x] }

// clone returns a copy of g that is not from the pool.
func (g *Grid) clone() *Grid {
	return &Grid{W: g.W, H: g.H, Cells: append([]gcell(nil), g.Cells...)}
}

// pixelCell is the cell showing two stacked pixels.
func pixelCell(top, bot RGB) gcell {
	qt, qb := top.q(), bot.q()
	if qt == qb {
		return gcell{ch: " ", fg: qt, bg: qb}
	}
	return gcell{ch: halfBlock, fg: qt, bg: qb}
}

// grid converts the scene into cells: pixels as half blocks, with the
// character layer on top.
func (s *Scene) grid() *Grid {
	g := newGrid(s.W, s.H)
	w := s.W
	for y := 0; y < s.H; y++ {
		top := s.Px.Pix[(2*y)*w : (2*y+1)*w]
		bot := s.Px.Pix[(2*y+1)*w : (2*y+2)*w]
		row := g.Cells[y*w : (y+1)*w]
		for x := range row {
			row[x] = pixelCell(top[x], bot[x])
		}
	}
	if s.cells != nil {
		for _, i := range s.used {
			cell := s.cells[i]
			if cell == nil {
				continue
			}
			x, y := i%w, i/w
			var fallback RGB
			if cell.Style.Bg == nil {
				fallback = Mix(s.Px.Pix[(2*y)*w+x], s.Px.Pix[(2*y+1)*w+x], 0.5)
			}
			g.setUV(x, y, cell, s.ink, fallback)
		}
	}
	return g
}

// setUV stores a Lip Gloss / Ultraviolet cell at (x, y). A cell without a
// foreground takes fg; without a background, bg.
func (g *Grid) setUV(x, y int, cell *uv.Cell, fg, bg RGB) {
	if x < 0 || y < 0 || x >= g.W || y >= g.H || cell == nil || cell.Width == 0 {
		return
	}
	c := gcell{ch: cell.Content, attrs: cell.Style.Attrs, ul: uint8(cell.Style.Underline)}
	if c.ch == "" {
		c.ch = " "
	}
	c.fg = colorQ(cell.Style.Fg, fg)
	c.bg = colorQ(cell.Style.Bg, bg)
	if cell.Width > 1 && x+1 < g.W {
		c.wide = true
		*g.at(x+1, y) = gcell{fg: c.fg, bg: c.bg}
	} else if cell.Width > 1 {
		c.ch = " " // a wide character that doesn't fit
	}
	// Overwriting half of a wide character breaks it: blank the other half.
	if old := g.at(x, y); old.ch == "" && x > 0 {
		if lead := g.at(x-1, y); lead.wide {
			lead.ch, lead.wide = " ", false
		}
	} else if old.wide && !c.wide && x+1 < g.W {
		g.at(x+1, y).ch = " "
	}
	*g.at(x, y) = c
}

func colorQ(c color.Color, fallback RGB) [3]uint8 {
	if c == nil {
		return fallback.q()
	}
	return toRGB(c).q()
}

// parseGrid draws a styled string (Lip Gloss output, or a frame string)
// into a fresh w×h grid on the theme's background.
func parseGrid(s string, w, h int, t *Theme) *Grid {
	g := blankGrid(w, h, t.Background)
	g.draw(0, 0, s, false, t.Text)
	return g
}

// draw places a styled block with its top-left corner at (x, y), with fg
// for text that has no color of its own. If transparent, unstyled spaces
// are skipped.
func (g *Grid) draw(x, y int, block string, transparent bool, fg RGB) {
	bw, bh := lipgloss.Size(block)
	if bw == 0 || bh == 0 {
		return
	}
	cv := lipgloss.NewCanvas(bw, bh)
	uv.NewStyledString(block).Draw(cv, cv.Bounds())
	for yy := 0; yy < bh; yy++ {
		ty := y + yy
		if ty < 0 || ty >= g.H {
			continue
		}
		for xx := 0; xx < bw; xx++ {
			tx := x + xx
			if tx < 0 || tx >= g.W {
				continue
			}
			cell := cv.CellAt(xx, yy)
			if cell == nil || cell.Width == 0 {
				continue
			}
			if transparent && (cell.Content == " " || cell.Content == "") && cell.Style.Bg == nil {
				continue
			}
			under := g.at(tx, ty)
			g.setUV(tx, ty, cell, fg, RGB{float32(under.bg[0]), float32(under.bg[1]), float32(under.bg[2])})
		}
	}
}

// String encodes the grid as exactly H lines of exactly W cells, with every
// cell's colors explicit. Colors are only written when they change.
func (g *Grid) String() string {
	var b strings.Builder
	b.Grow(g.W * g.H * 6)
	var p pen
	for y := 0; y < g.H; y++ {
		p.reset()
		row := g.Cells[y*g.W : (y+1)*g.W]
		for x := 0; x < len(row); x++ {
			c := &row[x]
			if c.ch == "" {
				continue // drawn by the wide character before it
			}
			buf := p.cell(nil, c)
			b.Write(buf)
			if c.wide {
				x++
			}
		}
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
	scratch    [64]byte
}

func (p *pen) reset() { *p = pen{} }

// cell appends the escape codes and text for c to buf (or to a scratch
// buffer when buf is nil) and returns it.
func (p *pen) cell(buf []byte, c *gcell) []byte {
	if buf == nil {
		buf = p.scratch[:0]
	}
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
