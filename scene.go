package decker

import (
	"image/color"
	"strings"

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

	ink          RGB           // character-layer text with no color of its own
	themeOverlay func(*Pixels) // drawn over the pixels by Render (Theme.Overlay)
	sink         *frameSink    // set by Ctx.Scene while the engine is capturing the slide
}

// A 682×171 scene is ~2.8MB of pixels and one is made every frame: recycle them
// (Render releases its scene).
var scenes = sizedPool[Scene]{max: 4}

// NewScene returns an empty w×h-cell scene on the theme's background. Slides
// use Ctx.Scene.
func NewScene(w, h int, t *Theme) *Scene {
	w, h = max(w, 1), max(h, 1)
	if s := scenes.get(func(s *Scene) bool { return s.W == w && s.H == h }); s != nil {
		s.Px.Fill(t.Background)
		s.Px.BG, s.ink = t.Background, t.Text
		return s
	}
	return &Scene{W: w, H: h, Px: NewPixels(w, 2*h, t.Background), ink: t.Text}
}

// Release returns s to the pool without rendering. Don't use s afterwards or
// after Render, which releases it.
func (s *Scene) Release() {
	s.themeOverlay, s.sink = nil, nil
	for _, i := range s.used {
		s.cells[i] = nil
	}
	s.used = s.used[:0]
	scenes.put(s)
}

func (s *Scene) setCell(x, y int, c *uv.Cell) {
	if s.cells == nil {
		s.cells = make([]*uv.Cell, s.W*s.H)
	}
	i := y*s.W + x
	if s.cells[i] == nil {
		s.used = append(s.used, i)
	}
	s.cells[i] = c
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

// runes draws the non-space runes of line from (x, y), styled by style(i).
func (s *Scene) runes(x, y int, line string, style func(i int) uv.Style) {
	col := 0
	for i, r := range []rune(line) {
		ch := string(r)
		if r != ' ' {
			s.Cell(x+col, y, ch, style(i))
		}
		col += max(ansi.StringWidth(ch), 1)
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

// Render returns the scene as exactly H lines of W cells, each with an explicit
// background, and releases it. While the engine is capturing, the slide's own
// scene (from Ctx.Scene) hands it the frame instead and Render returns "".
func (s *Scene) Render() string {
	if s.themeOverlay != nil {
		s.themeOverlay(s.Px)
	}
	k := s.sink
	if k != nil && (s.W != k.w || s.H != k.h) {
		k = nil // a scene from a resized Ctx is not the frame
	}
	if k != nil && k.pixels != nil {
		k.pixels(s.Px)
		s.Release()
		return ""
	}
	g := s.toGrid()
	s.Release()
	if k != nil && k.grid != nil {
		k.grid(g)
		return ""
	}
	out := g.String()
	g.release()
	return out
}
