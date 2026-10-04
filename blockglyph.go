package decker

import "math"

// quadrants maps block characters to the quarters they fill:
// bit 3 top-left, 2 top-right, 1 bottom-left, 0 bottom-right.
var quadrants = map[rune]uint8{
	'█': 0b1111, '▀': 0b1100, '▄': 0b0011, '▌': 0b1010, '▐': 0b0101,
	'▘': 0b1000, '▝': 0b0100, '▖': 0b0010, '▗': 0b0001,
	'▛': 0b1110, '▜': 0b1101, '▙': 0b1011, '▟': 0b0111,
	'▚': 0b1001, '▞': 0b0110,
}

// blockShades maps shade characters to the opacity they fill the cell with.
var blockShades = map[rune]float64{'░': 0.35, '▒': 0.55, '▓': 0.8}

type blockArms struct{ u, d, l, r, double bool }

// blockBoxArms maps box-drawing characters to the arms they draw. Joins of
// single and double lines (╒ ╕ ╘ ╛ ╥ ╨) are drawn as single lines.
var blockBoxArms = map[rune]blockArms{
	'─': {l: true, r: true}, '│': {u: true, d: true},
	'┌': {r: true, d: true}, '┐': {l: true, d: true}, '└': {u: true, r: true}, '┘': {u: true, l: true},
	'├': {u: true, d: true, r: true}, '┤': {u: true, d: true, l: true},
	'┬': {l: true, r: true, d: true}, '┴': {l: true, r: true, u: true},
	'┼': {u: true, d: true, l: true, r: true},
	'═': {l: true, r: true, double: true}, '║': {u: true, d: true, double: true},
	'╔': {r: true, d: true, double: true}, '╗': {l: true, d: true, double: true},
	'╚': {u: true, r: true, double: true}, '╝': {u: true, l: true, double: true},
	'╠': {u: true, d: true, r: true, double: true}, '╣': {u: true, d: true, l: true, double: true},
	'╦': {l: true, r: true, d: true, double: true}, '╩': {l: true, r: true, u: true, double: true},
	'╬': {u: true, d: true, l: true, r: true, double: true},
	'╒': {r: true, d: true}, '╕': {l: true, d: true}, '╘': {u: true, r: true}, '╛': {u: true, l: true},
	'╥': {l: true, r: true, d: true}, '╨': {l: true, r: true, u: true},
}

// isSolid reports whether r is a filled block character, which takes the
// block's color and glows, as opposed to a box-drawing edge or small mark.
func isSolid(r rune) bool { return solidRunes[r] }

var solidRunes = func() map[rune]bool {
	m := map[rune]bool{'■': true}
	for r := range quadrants {
		m[r] = true
	}
	for r := range blockShades {
		m[r] = true
	}
	return m
}()

// drawBlockRunePx paints one block or box-drawing character into the k×2k
// pixel box at (x, y).
func drawBlockRunePx(p *Pixels, r rune, x, y, k float64, c RGB, a float64) {
	cw, ch := k, 2*k
	if m, ok := quadrants[r]; ok {
		// Snap to whole pixels so neighboring cells tile exactly: with
		// antialiased fractional edges, two half-covered pixels at a shared
		// edge don't add up to a solid one, and a faint grid shows through.
		x0, x1 := math.Round(x), math.Round(x+cw)
		y0, y1 := math.Round(y), math.Round(y+ch)
		xm, ym := math.Round(x+cw/2), math.Round(y+ch/2)
		if m == 0b1111 {
			p.Rect(x0, y0, x1-x0, y1-y0, c, a)
			return
		}
		quads := [4][4]float64{{x0, y0, xm, ym}, {xm, y0, x1, ym}, {x0, ym, xm, y1}, {xm, ym, x1, y1}}
		for i, q := range quads {
			if m&(0b1000>>i) != 0 {
				p.Rect(q[0], q[1], q[2]-q[0], q[3]-q[1], c, a)
			}
		}
		return
	}
	x, y = math.Round(x), math.Round(y)
	if shade, ok := blockShades[r]; ok {
		p.Rect(x, y, cw, ch, c, a*shade)
		return
	}
	switch r {
	case '■', '◆':
		p.Rect(x+cw*0.2, y+ch*0.3, cw*0.6, ch*0.4, c, a)
		return
	case '·':
		p.Rect(x+cw*0.35, y+ch*0.42, cw*0.3, ch*0.16, c, a)
		return
	}
	arms, ok := blockBoxArms[r]
	if !ok {
		return
	}
	t := math.Max(1, k*0.3)
	cx, cy := x+cw/2, y+ch/2
	stroke := func(ox, oy float64) {
		if arms.l {
			p.Rect(x, cy+oy-t/2, cw/2+ox+t/2, t, c, a)
		}
		if arms.r {
			p.Rect(cx+ox-t/2, cy+oy-t/2, cw/2-ox+t/2, t, c, a)
		}
		if arms.u {
			p.Rect(cx+ox-t/2, y, t, ch/2+oy+t/2, c, a)
		}
		if arms.d {
			p.Rect(cx+ox-t/2, cy+oy-t/2, t, ch/2-oy+t/2, c, a)
		}
	}
	if arms.double {
		o := math.Max(k*0.22, t*0.8)
		stroke(-o, -o)
		stroke(o, o)
		return
	}
	stroke(0, 0)
}
