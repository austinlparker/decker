package decker

import (
	"fmt"
	"strconv"
)

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
	sc.drawing = "View"
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
		s.drawing = "a placed element"
		for _, e := range s.placed {
			if s.ctx.review == nil {
				e.draw(s.Px, e.r)
				continue
			}
			done := s.ctx.within("Place "+strconv.Quote(e.key), e.r)
			s.drawOwned(e)
			done()
		}
		s.overlay()
	})
}

// drawOwned draws a placed element under review and records the pixels it
// changed as its own, so a placed element built from shapes, which report
// no ink of their own, still overlaps what it covers. Like a morph, it looks
// a tenth of the canvas's height around the element's rect.
func (s *Scene) drawOwned(e placed) {
	l, p := s.Px.review, s.Px
	m := float64(p.H) / 10
	x0, y0, x1, y1 := p.Box(e.r.X-m, e.r.Y-m, e.r.Right()+m, e.r.Bottom()+m)
	if x0 > x1 || y0 > y1 {
		e.draw(p, e.r)
		return
	}
	before := snapshot(p, x0, y0, x1, y1)
	defer layers.put(before)
	e.draw(p, e.r)
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			if i := y*p.W + x; p.Pix[i] != before.Pix[i] {
				l.inkAt(l.scopeID, x, y)
			}
		}
	}
}

func (s *Scene) overlay() {
	if s.slide && s.ctx.Theme.Overlay != nil {
		if l := s.Px.review; l != nil {
			l.overlay = true
			defer func() { l.overlay = false }()
		}
		s.drawing = "Theme.Overlay"
		s.ctx.Theme.Overlay(s.ctx, s.Px)
	}
}

// safely runs f, which draws on s; if f panics, a slide's scene shows the
// panic instead of whatever was drawn, and keeps it for err. Off-screen
// scenes let it through to the View that made them.
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
	s.finished, s.fault = true, r
	c := s.ctx
	if l := s.Px.review; l != nil {
		l.overlay, l.scope = false, ""
		l.add(SeverityError, "panic", Rect{}, fmt.Sprint(r))
	}
	f := c.Theme.Mono
	Text{Font: f, Size: c.SmallText(f), Color: c.Theme.Warn, MaxW: c.X(0.9)}.
		Draw(s.Px, fmt.Sprintf("slide %q panicked:\n\n%v", s.title, r), c.X(0.05), c.Y(0.05))
}

// err is the panic the scene showed as an error naming slide i (0-based)
// and the part of it that panicked, or nil if nothing did. A panic value
// that is an error is wrapped.
func (s *Scene) err(i int) error {
	switch r := s.fault.(type) {
	case nil:
		return nil
	case error:
		return fmt.Errorf("slide %d %q: %s panicked: %w", i+1, s.title, s.drawing, r)
	default:
		return fmt.Errorf("slide %d %q: %s panicked: %v", i+1, s.title, s.drawing, r)
	}
}

// renderSlideGrid is renderSlide as cells; the caller releases the grid.
func renderSlideGrid(s Slide, c Ctx) *grid {
	return renderSlide(s, c).flatten()
}
