package decker

// Transition is how a slide enters. Each kind but Default and None needs an
// entry in transitions.
type Transition int

const (
	TransitionDefault  Transition = iota // use DefaultTransition
	TransitionNone                       // hard cut
	TransitionPush                       // new slide pushes the old one sideways
	TransitionDissolve                   // cells flip from old to new at random
	TransitionWipe                       // a bright edge sweeps across
)

// DefaultTransition applies to slides that don't set Transition.
var DefaultTransition = TransitionPush

// TransitionDuration is how long every transition takes, in seconds.
const TransitionDuration = 0.45

func (k Transition) resolve() Transition {
	if k == TransitionDefault {
		return DefaultTransition
	}
	return k
}

// transitionImpl draws a transition into cells and into video pixels. In both,
// p is linear progress from 0 (all old) to 1 (all new); each applies its own
// easing.
type transitionImpl struct {
	// cells fills out; forward is false when going backwards.
	cells func(out, from, to *grid, p float64, forward bool, t *Theme)
	// pixels mixes from into to in place, as packed RGB w×h frames.
	pixels func(from, to []byte, w, h int, p float64, t *Theme)
}

var transitions = map[Transition]transitionImpl{
	TransitionPush:     {pushCells, pushPixels},
	TransitionDissolve: {dissolveCells, dissolvePixels},
	TransitionWipe:     {wipeCells, wipePixels},
}

// composeGrid mixes two same-size frames into a new grid. Kinds without an
// implementation cut to `to`.
func composeGrid(kind Transition, from, to *grid, p float64, forward bool, t *Theme) *grid {
	out := newGrid(to.W, to.H)
	if impl, ok := transitions[kind]; ok {
		impl.cells(out, from, to, p, forward, t)
	} else {
		copy(out.Cells, to.Cells)
	}
	return out
}

// blendTransition mixes `from` into `to` in place. Kinds without an
// implementation leave `to` alone.
func blendTransition(kind Transition, from, to []byte, w, h int, p float64, t *Theme) {
	if impl, ok := transitions[kind]; ok {
		impl.pixels(from, to, w, h, p, t)
	}
}

func pushCells(out, from, to *grid, p float64, forward bool, _ *Theme) {
	w, h := to.W, to.H
	off := LerpInt(0, w, EaseInOutCubic(p))
	for y := 0; y < h; y++ {
		a, b, o := from.Cells[y*w:(y+1)*w], to.Cells[y*w:(y+1)*w], out.Cells[y*w:(y+1)*w]
		if forward {
			copy(o, a[off:])
			copy(o[w-off:], b[:off])
		} else {
			copy(o, b[w-off:])
			copy(o[off:], a[:w-off])
		}
		fixWideEdges(o)
	}
}

func dissolveCells(out, from, to *grid, p float64, _ bool, _ *Theme) {
	w, h := to.W, to.H
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
}

func wipeCells(out, from, to *grid, p float64, forward bool, t *Theme) {
	const glow = 6
	w, h := to.W, to.H
	edge := LerpInt(-glow, w+glow, EaseInOutCubic(p))
	shades := [...]string{"█", "▓", "▒", "░"}
	acc, bg := t.Accent, t.Background
	var shadeCells [len(shades)]gcell
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
}

// fixWideEdges blanks wide-character halves orphaned by cutting two frames
// together.
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

func pushPixels(from, to []byte, w, h int, p float64, _ *Theme) {
	off := int(float64(w) * EaseInOutCubic(p))
	row := make([]byte, 3*w)
	for y := 0; y < h; y++ {
		r := y * 3 * w
		copy(row, from[r+3*off:r+3*w])
		copy(row[3*(w-off):], to[r:r+3*off])
		copy(to[r:r+3*w], row)
	}
}

// dissolvePixels flips blocks the size of the deck's cells at random.
func dissolvePixels(from, to []byte, w, h int, p float64, _ *Theme) {
	bs := max(w/400, 1)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if Hash01(x/bs, y/(2*bs), 7) >= p {
				k := 3 * (y*w + x)
				copy(to[k:k+3], from[k:k+3])
			}
		}
	}
}

func wipePixels(from, to []byte, w, h int, p float64, t *Theme) {
	band := float64(w) / 60
	edge := -band + (float64(w)+2*band)*EaseInOutCubic(p)
	acc, bg := t.Accent, t.Background
	for x := 0; x < w; x++ {
		d := edge - float64(x) // how far behind the edge this column is
		if d >= band {
			continue
		}
		col := Mix(acc, bg, d/band).q()
		for y := 0; y < h; y++ {
			k := 3 * (y*w + x)
			if d < 0 {
				copy(to[k:k+3], from[k:k+3])
			} else {
				copy(to[k:k+3], col[:])
			}
		}
	}
}
