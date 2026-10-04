package decker

import (
	"image/color"

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
var grids = sizedPool[Grid]{max: 6}

// newGrid returns a w×h grid. Its cells are not cleared: callers fill every
// cell.
func newGrid(w, h int) *Grid {
	w, h = max(w, 1), max(h, 1)
	if g := grids.get(func(g *Grid) bool { return g.W == w && g.H == h }); g != nil {
		return g
	}
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
	if g != nil {
		grids.put(g)
	}
}

func (g *Grid) at(x, y int) *gcell { return &g.Cells[y*g.W+x] }

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
	for y := range s.H {
		top := s.Px.Pix[(2*y)*w : (2*y+1)*w]
		bot := s.Px.Pix[(2*y+1)*w : (2*y+2)*w]
		row := g.Cells[y*w : (y+1)*w]
		for x := range row {
			row[x] = pixelCell(top[x], bot[x])
		}
	}
	ink := s.ink.q()
	for _, i := range s.used {
		x, y := i%w, i/w
		cell := s.cells[i]
		var fallback [3]uint8
		if cell.Style.Bg == nil {
			fallback = Mix(s.Px.Pix[(2*y)*w+x], s.Px.Pix[(2*y+1)*w+x], 0.5).q()
		}
		g.setUV(x, y, cell, ink, fallback)
	}
	return g
}

// setUV stores a Lip Gloss / Ultraviolet cell at (x, y), which the caller
// has already clipped. A cell without a foreground takes fg; without a
// background, bg.
func (g *Grid) setUV(x, y int, cell *uv.Cell, fg, bg [3]uint8) {
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

func colorQ(c color.Color, fallback [3]uint8) [3]uint8 {
	if c == nil {
		return fallback
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
	q := fg.q()
	blit(block, x, y, g.W, g.H, transparent, func(tx, ty int, c *uv.Cell) {
		g.setUV(tx, ty, c, q, g.at(tx, ty).bg)
	})
}

// blit parses a styled block and calls set for each visible cell that lands
// inside a w×h area when the block's top-left corner is at (x, y). If
// transparent, unstyled spaces are skipped. set must not keep c.
func blit(block string, x, y, w, h int, transparent bool, set func(tx, ty int, c *uv.Cell)) {
	bw, bh := lipgloss.Size(block)
	if bw == 0 || bh == 0 {
		return
	}
	cv := lipgloss.NewCanvas(bw, bh)
	uv.NewStyledString(block).Draw(cv, cv.Bounds())
	for yy := range bh {
		ty := y + yy
		if ty < 0 || ty >= h {
			continue
		}
		for xx := range bw {
			tx := x + xx
			if tx < 0 || tx >= w {
				continue
			}
			c := cv.CellAt(xx, yy)
			if c == nil || c.Width == 0 {
				continue // continuation of a wide character
			}
			if transparent && (c.Content == " " || c.Content == "") && c.Style.Bg == nil {
				continue
			}
			set(tx, ty, c)
		}
	}
}
