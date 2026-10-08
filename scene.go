package decker

import (
	"image/color"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// Scene is a fixed-size grid of terminal cells that a slide draws into, layered
// in drawing order. Off-screen drawing is clipped, so animating in from the
// edges is safe. Two layers:
//
//   - Px, a pixel framebuffer twice as tall as the scene, for big type, shapes,
//     glows and pixel art.
//   - Character cells on top (Put, PutCenter, Overlay, Text, Cell, Fill,
//     Sprite), for small native-terminal text. A cell replaces the two pixels
//     beneath it; with no background of its own it takes their average color.
type Scene struct {
	W, H  int
	Px    *Pixels
	cells []*uv.Cell // character-layer cells; nil where the pixels show
	used  []int      // indexes of set cells, for clearing on reuse

	ink RGB // character-layer text with no color of its own

	moved []movedCell // moveChars' scratch

	placed []placed // elements to draw after View, in order (Place)
	pair   []int    // morph's scratch: the old element each new one matches
	taken  []bool   // morph's scratch: old elements already matched

	// A slide's scene (drawSlide) knows its slide and Ctx, for the overlay
	// and a panic's message; finished is set once its elements and overlay
	// are drawn.
	slide, finished bool
	ctx             Ctx
	title           string
}

type movedCell struct {
	i    int
	cell *uv.Cell
}

// A 682×171 scene is ~2.8MB of pixels and one is made every frame: recycle them
// (the engine releases a slide's scene, and Render its own).
var scenes = sizedPool[Scene]{max: 4}

// NewScene returns an empty w×h-cell scene on the theme's background, for
// drawing off-screen; a slide draws on the scene View is given.
func NewScene(w, h int, t *Theme) *Scene {
	w, h = max(w, 1), max(h, 1)
	if s := scenes.get(func(s *Scene) bool { return s.W == w && s.H == h }); s != nil {
		s.clear(t)
		return s
	}
	return &Scene{W: w, H: h, Px: NewPixels(w, 2*h, t.Background), ink: t.Text}
}

// clear empties s onto the theme's background: no pixels drawn, no
// characters, nothing placed.
func (s *Scene) clear(t *Theme) {
	s.Px.Fill(t.Background)
	s.Px.BG, s.ink = t.Background, t.Text
	s.clearChars()
	clear(s.placed)
	s.placed = s.placed[:0]
}

func (s *Scene) clearChars() {
	for _, i := range s.used {
		s.cells[i] = nil
	}
	s.used = s.used[:0]
}

// Release returns s to the pool without rendering. Don't use s afterwards or
// after Render, which releases it.
func (s *Scene) Release() {
	s.clearChars()
	clear(s.placed)
	s.placed = s.placed[:0]
	s.slide, s.finished, s.ctx, s.title = false, false, Ctx{}, ""
	s.Px.review = nil
	scenes.put(s)
}

func (s *Scene) setCell(x, y int, c *uv.Cell) {
	if s.cells == nil {
		s.cells = make([]*uv.Cell, s.W*s.H)
	}
	i := y*s.W + x
	// Resolve overlaps while drawing, so conversion order cannot resurrect
	// a glyph whose lead was written earlier than its other column.
	if x > 0 {
		if left := s.cells[i-1]; left != nil && left.Width > 1 {
			s.storeCell(i-1, &uv.Cell{Content: " ", Width: 1, Style: left.Style})
		}
	}
	if old := s.cells[i]; old != nil && old.Width > 1 && x+1 < s.W {
		s.storeCell(i+1, &uv.Cell{Content: " ", Width: 1, Style: old.Style})
	}
	if c.Width > 1 {
		if x+1 == s.W {
			c = &uv.Cell{Content: " ", Width: 1, Style: c.Style}
		} else {
			if next := s.cells[i+1]; next != nil && next.Width > 1 && x+2 < s.W {
				s.storeCell(i+2, &uv.Cell{Content: " ", Width: 1, Style: next.Style})
			}
			s.storeCell(i+1, &uv.Cell{Style: c.Style})
		}
	}
	s.storeCell(i, c)
}

func (s *Scene) storeCell(i int, c *uv.Cell) {
	if s.cells[i] == nil {
		s.used = append(s.used, i)
	}
	s.cells[i] = c
}

// moveChars rebuilds to's character layer from the cells of both frames:
// where says where the cell at (x, y) lands, from the old frame or the new
// one, and false if it doesn't show. Cells are shared, not copied; neither
// frame changes them.
func moveChars(from, to *Scene, where func(x, y int, old bool) (int, int, bool)) {
	if len(from.used) == 0 && len(to.used) == 0 {
		return
	}
	to.moved = to.moved[:0]
	for _, i := range to.used {
		if c := to.cells[i]; c.Width > 0 {
			to.moved = append(to.moved, movedCell{i, c})
		}
	}
	to.clearChars()
	place := func(i int, c *uv.Cell, old bool) {
		if x, y, ok := where(i%to.W, i/to.W, old); ok && x >= 0 && y >= 0 && x < to.W && y < to.H {
			if to.cells == nil {
				to.cells = make([]*uv.Cell, to.W*to.H)
			}
			// These glyphs already resolved drawing overlaps in their source
			// scene. Movement shares them without allocating replacements;
			// toGrid resolves overlaps where the two frames meet.
			to.storeCell(y*to.W+x, c)
		}
	}
	for _, i := range from.used {
		if c := from.cells[i]; c.Width > 0 {
			place(i, c, true)
		}
	}
	for _, m := range to.moved {
		place(m.i, m.cell, false)
	}
}

// Put draws a styled, possibly multi-line block at (x, y). Every cell is
// opaque, so spaces overwrite what was beneath.
func (s *Scene) Put(x, y int, block string) { s.put(x, y, block, false) }

// Overlay is Put with unstyled spaces transparent.
func (s *Scene) Overlay(x, y int, block string) { s.put(x, y, block, true) }

// PutCenter draws block centered in the scene, shifted by (dx, dy).
func (s *Scene) PutCenter(dx, dy int, block string) {
	w, h := lipgloss.Size(block)
	s.Put((s.W-w)/2+dx, (s.H-h)/2+dy, block)
}

func (s *Scene) put(x, y int, block string, transparent bool) {
	blit(block, x, y, s.W, s.H, transparent, func(tx, ty int, c *uv.Cell) { s.setCell(tx, ty, c.Clone()) })
}

// Text draws plain text in one color with transparent spaces; attrs are
// uv.Attr* bits.
func (s *Scene) Text(x, y int, text string, fg color.Color, attrs ...uint8) {
	var a uint8
	for _, v := range attrs {
		a |= v
	}
	st := uv.Style{Fg: fg, Attrs: a}
	for row, line := range strings.Split(text, "\n") {
		s.runes(x, y+row, line, func(int) uv.Style { return st })
	}
}

// runes draws graphemes from line, styled by their first rune's index so
// Sprite.Paint continues to address runes even when several form one glyph.
func (s *Scene) runes(x, y int, line string, style func(i int) uv.Style) {
	col, i := 0, 0
	for len(line) > 0 {
		ch, width := ansi.FirstGraphemeCluster(line, ansi.GraphemeWidth)
		if ch != " " && width > 0 {
			s.Cell(x+col, y, ch, style(i))
		}
		col += width
		i += utf8.RuneCountInString(ch)
		line = line[len(ch):]
	}
}

// Cell sets a single cell; out-of-bounds is ignored.
func (s *Scene) Cell(x, y int, ch string, st uv.Style) {
	if x < 0 || y < 0 || x >= s.W || y >= s.H {
		return
	}
	s.setCell(x, y, &uv.Cell{Content: ch, Width: max(ansi.StringWidth(ch), 1), Style: st})
}

// Fill paints a rectangle with a background color.
func (s *Scene) Fill(x, y, w, h int, bg color.Color) {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			s.Cell(xx, yy, " ", uv.Style{Bg: bg})
		}
	}
}

// Sprite is multi-line ASCII art with per-character colors; spaces are
// transparent. Each character of Paint (optional, same shape as Art) selects a
// color from Colors for the matching Art character; others use Default. For
// pixel art, see PixelArt.
//
//	bee := Sprite{
//	    Art:    []string{`(o)##>`},
//	    Paint:  []string{`kkkyyk`},
//	    Colors: map[rune]color.Color{'k': Hex("#222222").Color(), 'y': theme.Accent.Color()},
//	}
type Sprite struct {
	Art     []string
	Paint   []string
	Colors  map[rune]color.Color
	Default color.Color
	Bold    bool
}

// Size returns the sprite's width and height in cells.
func (sp Sprite) Size() (w, h int) {
	for _, l := range sp.Art {
		w = max(w, ansi.StringWidth(l))
	}
	return w, len(sp.Art)
}

// Sprite draws sp with its top-left corner at (x, y).
func (s *Scene) Sprite(x, y int, sp Sprite) {
	var attrs uint8
	if sp.Bold {
		attrs = uv.AttrBold
	}
	for row, line := range sp.Art {
		var paint []rune
		if row < len(sp.Paint) {
			paint = []rune(sp.Paint[row])
		}
		s.runes(x, y+row, line, func(i int) uv.Style {
			fg := sp.Default
			if i < len(paint) {
				if pc, ok := sp.Colors[paint[i]]; ok {
					fg = pc
				}
			}
			return uv.Style{Fg: fg, Attrs: attrs}
		})
	}
}

// Place adds an element to the scene: draw paints it into r after View
// returns, above View's own drawing, in the order placed. draw should size
// itself from r and stay inside it, give or take a glow, and must be
// frame-pure like View.
//
// key names the element across slides. When a slide enters with
// TransitionMorph, an element whose key the slide before it also placed
// glides from its old rect to its new one, cross-fading between the old
// draw and the new; elements on only one side fade out or in. An empty key
// never matches. Keys should be unique within a slide; repeats pair up in
// order.
func (s *Scene) Place(key string, r Rect, draw func(p *Pixels, r Rect)) {
	s.placed = append(s.placed, placed{key, r, draw})
}

type placed struct {
	key  string
	r    Rect
	draw func(p *Pixels, r Rect)
}

// Render returns the scene as exactly H lines of W cells, each with an explicit
// background, and releases it. Use it on scenes made with NewScene, never on
// the one a slide's View is given; placed elements are drawn first, but
// Theme.Overlay is not applied.
func (s *Scene) Render() string {
	g := s.flatten()
	defer g.release()
	return g.String()
}
