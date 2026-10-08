package decker

import "fmt"

// renderSlide draws s ready to show: its View, the elements it placed, then
// the theme's Overlay, once. A nil View is a blank slide. A panic is replaced
// by its message, drawn into the pixels so videos and PNGs show it too. The
// caller releases the scene.
func renderSlide(s Slide, c Ctx) *Scene {
	sc := drawSlide(s, c)
	sc.finish()
	return sc
}

// drawSlide runs s's View on a fresh scene. Elements it places are recorded,
// not drawn: finish draws them, or a morph moves them first.
func drawSlide(s Slide, c Ctx) *Scene {
	sc := NewScene(c.W, c.H, c.Theme)
	sc.slide, sc.ctx, sc.title = true, c, s.Title
	sc.Px.review = c.review
	if s.View != nil {
		sc.safely(func() { s.View(c, sc) })
	}
	return sc
}

// finish draws the scene's placed elements and, on a slide's scene, the
// theme's overlay. Finishing twice does nothing.
func (s *Scene) finish() {
	if s.finished {
		return
	}
	s.finished = true
	s.safely(func() {
		for _, e := range s.placed {
			e.draw(s.Px, e.r)
		}
		s.overlay()
	})
}

func (s *Scene) overlay() {
	if s.slide && s.ctx.Theme.Overlay != nil {
		if l := s.Px.review; l != nil {
			l.overlay = true
			defer func() { l.overlay = false }()
		}
		s.ctx.Theme.Overlay(s.ctx, s.Px)
	}
}

// safely runs f, which draws on s; if f panics, a slide's scene shows the
// panic instead of whatever was drawn. Off-screen scenes let it through to
// the View that made them.
func (s *Scene) safely(f func()) {
	if !s.slide {
		f()
		return
	}
	defer func() {
		if r := recover(); r != nil {
			s.showPanic(r)
		}
	}()
	f()
}

func (s *Scene) showPanic(r any) {
	s.clear(s.ctx.Theme)
	s.finished = true
	c := s.ctx
	if l := s.Px.review; l != nil {
		l.overlay, l.scope = false, ""
		l.add(SeverityError, "panic", Rect{}, fmt.Sprint(r))
	}
	f := c.Theme.Mono
	Text{Font: f, Size: c.SmallText(f), Color: c.Theme.Warn, MaxW: c.X(0.9)}.
		Draw(s.Px, fmt.Sprintf("slide %q panicked:\n\n%v", s.title, r), c.X(0.05), c.Y(0.05))
}

// renderSlideGrid is renderSlide as cells; the caller releases the grid.
func renderSlideGrid(s Slide, c Ctx) *grid {
	sc := renderSlide(s, c)
	g := sc.toGrid()
	sc.Release()
	return g
}
