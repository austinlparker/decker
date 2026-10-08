package decker

import (
	"image/color"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// grid is one finished frame of terminal cells: what the live deck writes and
// frame strings encode.
type grid struct {
	W, H  int
	Cells []gcell
}

// gcell is a space or half block (▀) with explicit colors, or a rich cell with
// text attributes.
type gcell struct {
	ch     string // one grapheme; "" for the second half of a wide character
	fg, bg [3]uint8
	attrs  uint8 // uv.Attr* bits
	ul     uint8 // uv.Underline style
	wide   bool  // ch takes two cells
}

const halfBlock = "▀"

// A 682×171 frame is ~117k cells and one is made every frame: recycle them.
var grids = sizedPool[grid]{max: 6}

// newGrid returns a w×h grid with uncleared cells: callers fill every cell.
func newGrid(w, h int) *grid {
	w, h = max(w, 1), max(h, 1)
	if g := grids.get(func(g *grid) bool { return g.W == w && g.H == h }); g != nil {
		return g
	}
	return &grid{W: w, H: h, Cells: make([]gcell, w*h)}
}

func blankGrid(w, h int, bg RGB) *grid {
	g := newGrid(w, h)
	q := bg.q()
	for i := range g.Cells {
		g.Cells[i] = gcell{ch: " ", fg: q, bg: q}
	}
	return g
}

// release returns g to the pool. Don't use it afterwards.
func (g *grid) release() {
	if g != nil {
		grids.put(g)
	}
}

func (g *grid) at(x, y int) *gcell { return &g.Cells[y*g.W+x] }

func pixelCell(top, bot RGB) gcell {
	if top == bot { // most cells are background
		q := top.q()
		return gcell{ch: " ", fg: q, bg: q}
	}
	qt, qb := top.q(), bot.q()
	if qt == qb {
		return gcell{ch: " ", fg: qt, bg: qb}
	}
	return gcell{ch: halfBlock, fg: qt, bg: qb}
}

// flatten finishes s, converts it to cells and releases it, as every frame
// that leaves as cells ends. The caller releases the grid.
func (s *Scene) flatten() *grid {
	s.finish()
	g := s.toGrid()
	s.Release()
	return g
}

// toGrid converts the scene to cells: pixels as half blocks, character layer on
// top.
func (s *Scene) toGrid() *grid {
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
		if cell.Width == 0 {
			continue // the lead writes both columns of a wide glyph
		}
		var fallback [3]uint8
		if cell.Style.Bg == nil {
			fallback = Mix(s.Px.Pix[(2*y)*w+x], s.Px.Pix[(2*y+1)*w+x], 0.5).q()
		}
		g.setUV(x, y, cell, ink, fallback)
	}
	return g
}

// setUV stores a cell at an already-clipped (x, y); a missing foreground takes
// fg, a missing background bg.
func (g *grid) setUV(x, y int, cell *uv.Cell, fg, bg [3]uint8) {
	c := gcell{ch: cell.Content, attrs: cell.Style.Attrs, ul: uint8(cell.Style.Underline)}
	if c.ch == "" {
		c.ch = " "
	}
	c.fg = colorQ(cell.Style.Fg, fg)
	c.bg = colorQ(cell.Style.Bg, bg)
	if cell.Width > 1 && x+1 < g.W {
		c.wide = true
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
	if c.wide {
		// The second column may itself be the lead of another wide glyph.
		// Erase its continuation before replacing that lead.
		if next := g.at(x+1, y); next.wide && x+2 < g.W {
			g.at(x+2, y).ch = " "
		}
		*g.at(x+1, y) = gcell{fg: c.fg, bg: c.bg}
	}
	*g.at(x, y) = c
}

func colorQ(c color.Color, fallback [3]uint8) [3]uint8 {
	if c == nil {
		return fallback
	}
	return toRGB(c).q()
}

// pixels is the grid as a canvas twice as tall: a half block is its
// foreground over its background, other characters blend the two.
func (g *grid) pixels() *Pixels {
	px := &Pixels{W: g.W, H: 2 * g.H, Pix: make([]RGB, 2*len(g.Cells))}
	for i := range g.Cells {
		c := &g.Cells[i]
		fg, bg := rgbOf(c.fg), rgbOf(c.bg)
		top, bot := bg, bg
		switch c.ch {
		case "", " ":
		case halfBlock:
			top = fg
		case "▄":
			bot = fg
		case "█":
			top, bot = fg, fg
		default:
			top = Mix(bg, fg, 0.5)
			bot = top
		}
		y := i / g.W // the cell's pixels are rows 2y and 2y+1
		px.Pix[i+y*g.W], px.Pix[i+(y+1)*g.W] = top, bot
	}
	return px
}

func rgbOf(q [3]uint8) RGB { return RGB{float32(q[0]), float32(q[1]), float32(q[2])} }

// draw places a styled block at (x, y); fg colors text with none. If
// transparent, unstyled spaces are skipped.
func (g *grid) draw(x, y int, block string, transparent bool, fg RGB) {
	q := fg.q()
	blit(block, x, y, g.W, g.H, transparent, func(tx, ty int, c *uv.Cell) {
		g.setUV(tx, ty, c, q, g.at(tx, ty).bg)
	})
}

// blit parses a styled block and calls set for each visible cell landing inside
// a w×h area with the block's top-left at (x, y). If transparent, unstyled
// spaces are skipped. set must not keep c.
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
