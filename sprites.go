package decker

// Pixel-art characters. Each frame is a grid of letters; each letter picks
// a color from the art's Colors and '.' (or any letter without a color) is
// transparent. They're drawn into the pixel layer at any scale (see
// Pixels.Art), so they stay crisp and big. Animate a character by returning
// a different frame for a different t.

// PixelArt is one frame of a character.
type PixelArt struct {
	Rows   []string
	Colors map[rune]RGB
}

// Size returns the art's size in pixels at scale 1.
func (a PixelArt) Size() (w, h int) {
	for _, r := range a.Rows {
		w = max(w, len(r))
	}
	return w, len(a.Rows)
}

// Art draws a with its top-left corner at (x, y), scaled by scale
// (fractional scales are fine), mirrored horizontally if flip is true.
func (p *Pixels) Art(a PixelArt, x, y, scale float64, alpha float64, flip bool) {
	aw, ah := a.Size()
	x0, y0, x1, y1 := p.Box(x, y, x+float64(aw)*scale, y+float64(ah)*scale)
	for py := y0; py <= y1; py++ {
		sy := int((float64(py) + 0.5 - y) / scale)
		if sy < 0 || sy >= ah {
			continue
		}
		row := a.Rows[sy]
		for px := x0; px <= x1; px++ {
			sx := int((float64(px) + 0.5 - x) / scale)
			if flip {
				sx = aw - 1 - sx
			}
			if sx < 0 || sx >= len(row) {
				continue
			}
			if c, ok := a.Colors[rune(row[sx])]; ok {
				p.Blend(px, py, c, alpha)
			}
		}
	}
}
