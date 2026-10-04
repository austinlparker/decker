package decker

import (
	"image/color"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// Scene is a fixed-size grid of terminal cells that a slide draws into.
// Think of it as a tiny 2D canvas: place styled text, Lip Gloss blocks, and
// sprites at exact (x, y) positions, layered in the order you draw them.
// Anything drawn partly or wholly off-screen is clipped, so it is safe to
// animate things in from beyond the edges.
//
// A scene has two layers:
//
//   - Px, a pixel framebuffer twice as tall as the scene (see pixels.go).
//     Big type, shapes, glows, and pixel-art characters go here.
//   - Character cells on top: Put, Overlay, Text, Cell, and Sprite. Use
//     these for small native-terminal text. A cell drawn here replaces the
//     two pixels beneath it; if it has no background color of its own, it
//     takes the average color of those pixels so it blends in.
type Scene struct {
	W, H  int
	Px    *Pixels
	cells []*uv.Cell // character-layer cells; nil where the pixels show
	used  []int      // indexes of set cells, for clearing on reuse

	ink     RGB           // character-layer text with no color of its own
	overlay func(*Pixels) // drawn over the pixels by Render (Theme.Overlay)
}

// Scenes are big (a 682×171 scene is ~2.8MB of pixels) and a slide makes a
// new one every frame, so they're recycled: Render returns its scene to
// the pool once the frame is encoded.
var (
	scenePoolMu sync.Mutex
	scenePool   []*Scene
)

// NewScene returns an empty scene of the given size in cells, on the
// theme's background. Slides get theirs from Ctx.Scene, which also adds the
// theme's overlay; use NewScene to draw something off screen.
func NewScene(w, h int, t *Theme) *Scene {
	w, h = max(w, 1), max(h, 1)
	bg := t.Background
	scenePoolMu.Lock()
	for i, s := range scenePool {
		if s.W == w && s.H == h {
			scenePool = append(scenePool[:i], scenePool[i+1:]...)
			scenePoolMu.Unlock()
			for j := range s.Px.Pix {
				s.Px.Pix[j] = bg
			}
			s.Px.BG, s.ink = bg, t.Text
			return s
		}
	}
	scenePoolMu.Unlock()
	return &Scene{W: w, H: h, Px: NewPixels(w, 2*h, bg), ink: t.Text}
}

// Release returns a scene to the pool without rendering it, once you've
// taken what you need from its pixels. Don't use it afterwards.
func (s *Scene) Release() { s.recycle() }

func (s *Scene) recycle() {
	s.overlay = nil
	for _, i := range s.used {
		s.cells[i] = nil
	}
	s.used = s.used[:0]
	scenePoolMu.Lock()
	if len(scenePool) < 4 {
		scenePool = append(scenePool, s)
	}
	scenePoolMu.Unlock()
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

// Put draws a (possibly styled, multi-line) string with its top-left corner
// at (x, y). Every cell of the block is opaque, so spaces overwrite whatever
// was underneath. Use it for Lip Gloss boxes, Markdown output, etc.
func (s *Scene) Put(x, y int, block string) { s.put(x, y, block, false) }

// Overlay is like Put, but unstyled spaces are transparent: only visible
// characters (and cells with a background color) are drawn.
func (s *Scene) Overlay(x, y int, block string) { s.put(x, y, block, true) }

// PutCenter draws block centered in the scene, shifted by (dx, dy).
func (s *Scene) PutCenter(dx, dy int, block string) {
	w, h := lipgloss.Size(block)
	s.Put((s.W-w)/2+dx, (s.H-h)/2+dy, block)
}

// Text draws plain text in one color with transparent spaces. Optional attrs
// are uv.AttrBold, uv.AttrItalic, uv.AttrFaint, etc., OR'd together.
func (s *Scene) Text(x, y int, text string, fg color.Color, attrs ...uint8) {
	var a uint8
	for _, v := range attrs {
		a |= v
	}
	st := uv.Style{Fg: fg, Attrs: a}
	for row, line := range strings.Split(text, "\n") {
		cx := x
		for _, r := range line {
			ch := string(r)
			w := ansi.StringWidth(ch)
			if r != ' ' {
				s.Cell(cx, y+row, ch, st)
			}
			cx += max(w, 1)
		}
	}
}

// Cell sets a single cell. Out-of-bounds coordinates are ignored.
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

// Render returns the scene as a styled string of exactly H lines, each
// exactly W cells wide, and recycles the scene: don't use it afterwards.
//
// Each cell is a half block (▀) whose foreground is the top pixel and
// background the bottom one, or a space when both match. Every cell gets an
// explicit background color: terminals with a translucent background only
// make the *default* background see-through, and the deck should be opaque.
// Colors are only written when they change, to keep frames small: at
// 682×171 a frame is 117k cells, and the terminal has to parse all of it.
func (s *Scene) Render() string {
	if s.overlay != nil {
		s.overlay(s.Px)
	}
	if captureFrame != nil {
		// Rendering a video: hand over the pixels, and skip encoding.
		captureFrame(s.Px)
		s.recycle()
		return ""
	}
	g := s.grid()
	s.recycle()
	if captureGrid != nil {
		// The live deck: hand over the cells, and skip encoding.
		captureGrid(g)
		return ""
	}
	out := g.String()
	g.release()
	return out
}

// captureFrame, when set, receives each scene's pixels in place of Render
// encoding them, before the scene is reused. Video rendering sets it; it
// isn't safe to use from more than one goroutine.
var captureFrame func(*Pixels)

// captureGrid, when set, receives each scene's cells in place of Render
// encoding them, and owns the grid. renderSlideGrid sets it; it isn't safe
// to use from more than one goroutine.
var captureGrid func(*Grid)

// opaque redraws a styled string (the footer, panels, an error message) as
// a w×h scene on the theme's background, so it gets an explicit background
// color everywhere, like the slides.
func opaque(s string, w, h int, t *Theme) string {
	sc := NewScene(w, h, t)
	sc.Put(0, 0, s)
	return sc.Render()
}

func (s *Scene) put(x, y int, block string, transparent bool) {
	w, h := lipgloss.Size(block)
	if w == 0 || h == 0 {
		return
	}
	tmp := lipgloss.NewCanvas(w, h)
	uv.NewStyledString(block).Draw(tmp, tmp.Bounds())
	for yy := 0; yy < h; yy++ {
		ty := y + yy
		if ty < 0 || ty >= s.H {
			continue
		}
		for xx := 0; xx < w; xx++ {
			tx := x + xx
			if tx < 0 || tx >= s.W {
				continue
			}
			cell := tmp.CellAt(xx, yy)
			if cell == nil || cell.Width == 0 {
				continue // continuation of a wide character
			}
			if transparent && (cell.Content == " " || cell.Content == "") && cell.Style.Bg == nil {
				continue
			}
			s.setCell(tx, ty, cell.Clone())
		}
	}
}

// Sprite is multi-line ASCII art with per-character colors. Spaces are
// transparent. Paint (optional) has the same shape as Art; each character
// in Paint selects a color from Colors for the matching Art character.
// Characters in Paint that are not in Colors (e.g. ' ') use Default.
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
		col := 0
		for i, r := range []rune(line) {
			if r != ' ' {
				fg := sp.Default
				if i < len(paint) {
					if pc, ok := sp.Colors[paint[i]]; ok {
						fg = pc
					}
				}
				s.Cell(x+col, y+row, string(r), uv.Style{Fg: fg, Attrs: attrs})
			}
			col += max(ansi.StringWidth(string(r)), 1)
		}
	}
}
