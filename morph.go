package decker

// morph is TransitionMorph. The two slides' own drawing (View's) cross-fades;
// then each element the new slide placed is drawn, gliding from the rect of
// the old slide's element with the same key if there is one, while the old
// slide's unmatched elements fade out and the new one's fade in. A slide
// whose elements were drawn already (a frame captured mid-transition) just
// cross-fades.
func morph(from, to *Scene, p float64, _ Direction, t *Theme) {
	e := EaseInOutCubic(p)
	blend(from.Px, to.Px, e)
	moveChars(from, to, func(x, y int, old bool) (int, int, bool) { return x, y, old == (e < 0.5) })

	var olds []placed
	if !from.finished {
		olds = from.placed
	}
	pair := to.pairScratch(len(to.placed), len(olds))
	for i, n := range to.placed {
		pair[i] = -1
		if n.key == "" {
			continue
		}
		for j, o := range olds {
			if o.key == n.key && !to.taken[j] {
				pair[i], to.taken[j] = j, true
				break
			}
		}
	}
	drawn := to.finished // its elements and the overlay too
	to.finished = true
	to.safely(func() {
		margin := to.Px.H / 10 // room for a glow around the rect
		for j, o := range olds {
			if !to.taken[j] {
				fade(to.Px, o.r, margin, 1-e, o.draw, nil)
			}
		}
		if drawn {
			return
		}
		for i, n := range to.placed {
			if j := pair[i]; j < 0 {
				fade(to.Px, n.r, margin, e, n.draw, nil)
			} else {
				fade(to.Px, LerpRect(olds[j].r, n.r, e), margin, e, n.draw, olds[j].draw)
			}
		}
		to.overlay()
	})
}

// pairScratch returns s's scratch for matching n new elements with m old
// ones, with every old one untaken.
func (s *Scene) pairScratch(n, m int) []int {
	if cap(s.pair) < n {
		s.pair = make([]int, n)
	}
	if cap(s.taken) < m {
		s.taken = make([]bool, m)
	}
	s.taken = s.taken[:m]
	clear(s.taken)
	return s.pair[:n]
}

// layers holds scratch canvases for fade, frame-sized like scenes.
var layers = sizedPool[Pixels]{max: 4}

// layer lends a canvas the size of like; drawing on it reports to like's
// review, since its coordinates are like's.
func layer(like *Pixels) *Pixels {
	if l := layers.get(func(l *Pixels) bool { return l.W == like.W && l.H == like.H }); l != nil {
		l.review = like.review
		return l
	}
	return &Pixels{W: like.W, H: like.H, Pix: make([]RGB, len(like.Pix)), review: like.review}
}

// fade mixes draw(r) into p at weight a, as if drawn on a layer of its own,
// within margin pixels of r. With under set, it instead mixes from under(r)
// to draw(r): one element turning into another, with no dip in between where
// the two look the same.
func fade(p *Pixels, r Rect, margin int, a float64, draw, under func(*Pixels, Rect)) {
	switch {
	case a <= 0 && under == nil:
		return
	case a <= 0:
		under(p, r)
		return
	case a >= 1:
		draw(p, r)
		return
	}
	m := float64(margin)
	x0, y0, x1, y1 := p.Box(r.X-m, r.Y-m, r.Right()+m, r.Bottom()+m)
	if x0 > x1 || y0 > y1 {
		return
	}
	snap := func(l *Pixels) {
		for y := y0; y <= y1; y++ {
			copy(l.Pix[y*p.W+x0:y*p.W+x1+1], p.Pix[y*p.W+x0:y*p.W+x1+1])
		}
		l.BG = p.BG
	}
	top := layer(p)
	defer layers.put(top)
	snap(top)
	draw(top, r)
	base := p
	if under != nil {
		base = layer(p)
		defer layers.put(base)
		snap(base)
		under(base, r)
	}
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			k := y*p.W + x
			p.Pix[k] = Mix(base.Pix[k], top.Pix[k], a)
		}
	}
}
