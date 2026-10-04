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

// boxArms maps box-drawing characters to their arms' weights in the order up,
// right, down, left: 0 none, 1 single, 2 double.
var boxArms = map[rune][4]uint8{
	'─': {0, 1, 0, 1}, '━': {0, 1, 0, 1}, '│': {1, 0, 1, 0}, '┃': {1, 0, 1, 0},
	'┌': {0, 1, 1, 0}, '┐': {0, 0, 1, 1}, '└': {1, 1, 0, 0}, '┘': {1, 0, 0, 1},
	'╭': {0, 1, 1, 0}, '╮': {0, 0, 1, 1}, '╰': {1, 1, 0, 0}, '╯': {1, 0, 0, 1},
	'├': {1, 1, 1, 0}, '┤': {1, 0, 1, 1}, '┬': {0, 1, 1, 1}, '┴': {1, 1, 0, 1}, '┼': {1, 1, 1, 1},
	'═': {0, 2, 0, 2}, '║': {2, 0, 2, 0},
	'╔': {0, 2, 2, 0}, '╗': {0, 0, 2, 2}, '╚': {2, 2, 0, 0}, '╝': {2, 0, 0, 2},
	'╠': {2, 2, 2, 0}, '╣': {2, 0, 2, 2}, '╦': {0, 2, 2, 2}, '╩': {2, 2, 0, 2}, '╬': {2, 2, 2, 2},
	'╒': {0, 2, 1, 0}, '╕': {0, 0, 1, 2}, '╘': {1, 2, 0, 0}, '╛': {1, 0, 0, 2},
	'╓': {0, 1, 2, 0}, '╖': {0, 0, 2, 1}, '╙': {2, 1, 0, 0}, '╜': {2, 0, 0, 1},
	'╞': {1, 2, 1, 0}, '╡': {1, 0, 1, 2}, '╤': {0, 2, 1, 2}, '╧': {1, 2, 0, 2},
	'╟': {2, 1, 2, 0}, '╢': {2, 0, 2, 1}, '╥': {0, 1, 2, 1}, '╨': {2, 1, 0, 1},
	'╪': {1, 2, 1, 2}, '╫': {2, 1, 2, 1},
	'╴': {0, 0, 0, 1}, '╵': {1, 0, 0, 0}, '╶': {0, 1, 0, 0}, '╷': {0, 0, 1, 0},
	'╸': {0, 0, 0, 1}, '╹': {1, 0, 0, 0}, '╺': {0, 1, 0, 0}, '╻': {0, 0, 1, 0},
}

// isSolid reports whether r is a filled block character (colored and glowing).
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

// drawBlockRunePx paints one block or box-drawing rune into the k×2k box at (x,
// y).
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
	arms, ok := boxArms[r]
	if !ok {
		return
	}
	// Mixed single and double joins are drawn as single lines.
	double := arms[0] != 1 && arms[1] != 1 && arms[2] != 1 && arms[3] != 1
	t := math.Max(1, k*0.3)
	cx, cy := x+cw/2, y+ch/2
	stroke := func(ox, oy float64) {
		if arms[3] > 0 {
			p.Rect(x, cy+oy-t/2, cw/2+ox+t/2, t, c, a)
		}
		if arms[1] > 0 {
			p.Rect(cx+ox-t/2, cy+oy-t/2, cw/2-ox+t/2, t, c, a)
		}
		if arms[0] > 0 {
			p.Rect(cx+ox-t/2, y, t, ch/2+oy+t/2, c, a)
		}
		if arms[2] > 0 {
			p.Rect(cx+ox-t/2, cy+oy-t/2, t, ch/2-oy+t/2, c, a)
		}
	}
	if double {
		o := math.Max(k*0.22, t*0.8)
		stroke(-o, -o)
		stroke(o, o)
		return
	}
	stroke(0, 0)
}
