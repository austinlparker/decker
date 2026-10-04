package decker

import "fmt"

// renderSlide draws s on a fresh scene, then the theme's Overlay, once. A nil
// View is a blank slide. A panicking View is replaced by its message, drawn
// into the pixels so videos and PNGs show it too. The caller releases the
// scene.
func renderSlide(s Slide, c Ctx) (sc *Scene) {
	sc = NewScene(c.W, c.H, c.Theme)
	defer func() {
		if r := recover(); r != nil {
			sc.Release()
			sc = NewScene(c.W, c.H, c.Theme)
			f := c.Theme.Mono
			Text{Font: f, Size: c.SmallText(f), Color: c.Theme.Warn, MaxW: c.X(0.9)}.
				Draw(sc.Px, fmt.Sprintf("slide %q panicked:\n\n%v", s.Title, r), c.X(0.05), c.Y(0.05))
		}
	}()
	if s.View != nil {
		s.View(c, sc)
	}
	if o := c.Theme.Overlay; o != nil {
		o(c, sc.Px)
	}
	return sc
}

// renderSlideGrid is renderSlide as cells; the caller releases the grid.
func renderSlideGrid(s Slide, c Ctx) *grid {
	sc := renderSlide(s, c)
	g := sc.toGrid()
	sc.Release()
	return g
}
