package decker

// PixelArt is one frame of a pixel-art character: each letter in Rows picks a
// color from Colors, and '.' (or any letter without a color) is transparent.
// Rows must be ASCII. Draw it at any scale with Pixels.Art, and animate by
// returning a different frame for a different t. (For small cell-based art in
// the character layer, see Sprite.)
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
// (fractional scales are fine) at opacity alpha, mirrored horizontally if flip
// is true.
func (p *Pixels) Art(a PixelArt, x, y, scale, alpha float64, flip bool) {
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
