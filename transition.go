package decker

// composeGrid mixes two full frames cell by cell into a new grid. p runs
// from 0 (all `from`) to 1 (all `to`). forward is false when navigating
// backwards, which mirrors directional transitions. Both frames must be the
// same size. The wipe's edge is the theme's accent.
func composeGrid(kind Transition, from, to *Grid, p float64, forward bool, t *Theme) *Grid {
	w, h := to.W, to.H
	out := newGrid(w, h)
	switch kind {
	case TransitionPush:
		off := LerpInt(0, w, EaseInOutCubic(p))
		for y := 0; y < h; y++ {
			a, b, o := from.Cells[y*w:(y+1)*w], to.Cells[y*w:(y+1)*w], out.Cells[y*w:(y+1)*w]
			if forward { // the old frame slides out to the left
				copy(o, a[off:])
				copy(o[w-off:], b[:off])
			} else { // ...and to the right
				copy(o, b[w-off:])
				copy(o[off:], a[:w-off])
			}
			fixWideEdges(o)
		}

	case TransitionDissolve:
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				i := y*w + x
				if Hash01(x, y, 7) < p {
					out.Cells[i] = to.Cells[i]
				} else {
					out.Cells[i] = from.Cells[i]
				}
			}
			fixWideEdges(out.Cells[y*w : (y+1)*w])
		}

	case TransitionWipe:
		const glow = 6
		edge := LerpInt(-glow, w+glow, EaseInOutCubic(p))
		shades := []string{"█", "▓", "▒", "░"}
		acc, bg := t.Accent, t.Background
		var shadeCells [4]gcell
		for d := range shadeCells {
			// Explicit background, so a translucent terminal doesn't show through.
			shadeCells[d] = gcell{ch: shades[d], fg: Mix(acc, bg, float64(d)/float64(len(shades))).q(), bg: bg.q()}
		}
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				i := y*w + x
				d := edge - x // how far behind the edge this cell is
				if !forward {
					d = x - (w - 1 - edge)
				}
				switch {
				case d >= 0 && d < len(shades):
					out.Cells[i] = shadeCells[d]
				case d >= len(shades):
					out.Cells[i] = to.Cells[i]
				default:
					out.Cells[i] = from.Cells[i]
				}
			}
			fixWideEdges(out.Cells[y*w : (y+1)*w])
		}

	default:
		copy(out.Cells, to.Cells)
	}
	return out
}

// fixWideEdges blanks halves of wide characters that lost their other
// half when two frames were cut together.
func fixWideEdges(row []gcell) {
	for x := range row {
		c := &row[x]
		switch {
		case c.ch == "" && (x == 0 || !row[x-1].wide):
			c.ch = " "
		case c.wide && (x+1 >= len(row) || row[x+1].ch != ""):
			c.ch, c.wide = " ", false
		}
	}
}

// composeTransition is composeGrid for frame strings.
func composeTransition(kind Transition, from, to string, w, h int, p float64, forward bool, t *Theme) string {
	if w <= 0 || h <= 0 {
		return to
	}
	a, b := parseGrid(from, w, h, t), parseGrid(to, w, h, t)
	out := composeGrid(kind, a, b, p, forward, t)
	s := out.String()
	a.release()
	b.release()
	out.release()
	return s
}
