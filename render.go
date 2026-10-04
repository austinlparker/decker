package decker

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// frameSink receives a slide's own scene (Ctx.Scene) in place of Scene.Render
// encoding it to a string (pixels if set, else grid; the last scene wins).
// Scenes made with NewScene are never captured.
type frameSink struct {
	pixels func(*Pixels)
	grid   func(*grid) // takes ownership of the grid
}

// renderSlideGrid draws one frame of s as cells, at exactly c.W × c.H.
func renderSlideGrid(s Slide, c Ctx) *grid {
	var got *grid
	c.sink = &frameSink{grid: func(g *grid) { got.release(); got = g }}
	out := renderSlide(s, c)
	if got != nil {
		return got
	}
	// The slide drew without a scene: decode its text.
	return parseGrid(out, max(c.W, 1), max(c.H, 1), c.Theme)
}

// renderSlide draws s as a string of exactly c.W × c.H cells. A nil View
// renders a blank slide; a panicking View renders the panic message.
func renderSlide(s Slide, c Ctx) (out string) {
	defer func() {
		if r := recover(); r != nil {
			out = opaque(c.Theme.styles().warn.Render(fmt.Sprintf("slide %q panicked:\n\n%v", s.Title, r)), c.W, c.H, c.Theme)
		}
	}()
	if s.View == nil {
		return opaque("", c.W, c.H, c.Theme)
	}
	return fit(s.View(c), c.W, c.H)
}

// opaque redraws a styled string (the footer, panels, an error message) as
// a w×h scene on the theme's background, so it gets an explicit background
// color everywhere, like the slides.
func opaque(s string, w, h int, t *Theme) string {
	sc := NewScene(w, h, t)
	sc.Put(0, 0, s)
	return sc.Render()
}

// fit clips or pads s to exactly w x h cells.
func fit(s string, w, h int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for i, l := range lines {
		if n, ok := narrowWidth(l); !ok || n > w {
			lines[i] = ansi.Truncate(l, w, "")
		}
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// narrowWidth is a fast width for lines made of escape sequences and
// single-width characters, which is what Scene.Render produces. It reports
// ok=false if the line might hold wide characters (CJK, emoji), so the
// caller can fall back to a full measurement. Checking every frame with
// ansi.StringWidth costs several milliseconds at big terminal sizes.
func narrowWidth(s string) (n int, ok bool) {
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == 0x1b:
			// Skip a CSI sequence: ESC [ params final-byte.
			if i+1 < len(s) && s[i+1] == '[' {
				i += 2
				for i < len(s) && (s[i] < 0x40 || s[i] > 0x7e) {
					i++
				}
				i++
				continue
			}
			return 0, false
		case c < 0x80:
			n++
			i++
		default:
			r, size := utf8.DecodeRuneInString(s[i:])
			// Box drawing, blocks, arrows, punctuation and the like are
			// narrow; anything past U+1100 outside that range might not be.
			if r >= 0x1100 && (r < 0x2000 || r > 0x2bff) {
				return 0, false
			}
			n++
			i += size
		}
	}
	return n, true
}
