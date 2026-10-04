package decker

import (
	"image"
	"image/color"
	"image/draw"
)

// drawBlockRune paints block elements and box-drawing characters into a
// preview cell the way a terminal draws them (the preview's bitmap font has
// none of them). It reports whether it handled the rune.
func drawBlockRune(img *image.RGBA, r rune, cell image.Rectangle, fg, bg color.Color) bool {
	fill := func(x0, y0, x1, y1 int, c color.Color) {
		draw.Draw(img, image.Rect(cell.Min.X+x0, cell.Min.Y+y0, cell.Min.X+x1, cell.Min.Y+y1), &image.Uniform{c}, image.Point{}, draw.Src)
	}
	w, h := cell.Dx(), cell.Dy()
	shade := func(p float64) color.Color { return Mix(toRGB(bg), toRGB(fg), p).Color() }

	switch r {
	case '█', '■':
		fill(0, 0, w, h, fg)
		return true
	case '▀':
		fill(0, 0, w, h/2, fg)
		return true
	case '▄':
		fill(0, h/2, w, h, fg)
		return true
	case '▌':
		fill(0, 0, w/2, h, fg)
		return true
	case '▐':
		fill(w/2, 0, w, h, fg)
		return true
	case '░':
		fill(0, 0, w, h, shade(0.25))
		return true
	case '▒':
		fill(0, 0, w, h, shade(0.5))
		return true
	case '▓':
		fill(0, 0, w, h, shade(0.75))
		return true
	}
	// Quadrants: bits are top-left, top-right, bottom-left, bottom-right.
	quads := map[rune]int{'▘': 8, '▝': 4, '▖': 2, '▗': 1, '▚': 9, '▞': 6, '▙': 11, '▛': 14, '▜': 13, '▟': 7}
	if q, ok := quads[r]; ok {
		if q&8 != 0 {
			fill(0, 0, w/2, h/2, fg)
		}
		if q&4 != 0 {
			fill(w/2, 0, w, h/2, fg)
		}
		if q&2 != 0 {
			fill(0, h/2, w/2, h, fg)
		}
		if q&1 != 0 {
			fill(w/2, h/2, w, h, fg)
		}
		return true
	}

	// Box drawing: which arms (up, right, down, left) the character has,
	// 1 = single line, 2 = double.
	arms, ok := boxArms[r]
	if !ok {
		return false
	}
	cx, cy := w/2, h/2
	line := func(dir, kind int) {
		offs := []int{0}
		if kind == 2 {
			offs = []int{-2, 2}
		}
		for _, o := range offs {
			switch dir {
			case 0: // up
				fill(cx+o-1, 0, cx+o+1, cy+1, fg)
			case 1: // right
				fill(cx, cy+o-1, w, cy+o+1, fg)
			case 2: // down
				fill(cx+o-1, cy-1, cx+o+1, h, fg)
			case 3: // left
				fill(0, cy+o-1, cx+1, cy+o+1, fg)
			}
		}
	}
	for dir, kind := range arms {
		if kind > 0 {
			line(dir, kind)
		}
	}
	return true
}

var boxArms = map[rune][4]int{
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
