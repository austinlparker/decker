package decker

import "math"

// Pixels is a true-color framebuffer. The scene shows it as half blocks: each
// terminal cell is two stacked pixels, so 240×60 cells is 240×120 pixels.
// Coordinates are floats, edges are antialiased, and drawing methods blend at
// opacity a in 0..1.
type Pixels struct {
	W, H int
	Pix  []RGB
	BG   RGB // the color the canvas was cleared to
}

// NewPixels returns a w×h framebuffer filled with bg.
func NewPixels(w, h int, bg RGB) *Pixels {
	p := &Pixels{W: w, H: h, Pix: make([]RGB, w*h), BG: bg}
	p.Fill(bg)
	return p
}

// At returns the pixel at (x, y), or black out of bounds.
func (p *Pixels) At(x, y int) RGB {
	if x < 0 || y < 0 || x >= p.W || y >= p.H {
		return RGB{}
	}
	return p.Pix[y*p.W+x]
}

// Set overwrites a pixel.
func (p *Pixels) Set(x, y int, c RGB) {
	if x >= 0 && y >= 0 && x < p.W && y < p.H {
		p.Pix[y*p.W+x] = c
	}
}

// Blend paints c over pixel (x, y) with opacity a.
func (p *Pixels) Blend(x, y int, c RGB, a float64) {
	if a <= 0 || x < 0 || y < 0 || x >= p.W || y >= p.H {
		return
	}
	i := y*p.W + x
	if a >= 1 {
		p.Pix[i] = c
		return
	}
	p.Pix[i] = Mix(p.Pix[i], c, a)
}

// Add brightens pixel (x, y) by c×a, clamped: additive light for glows.
func (p *Pixels) Add(x, y int, c RGB, a float64) {
	if a <= 0 || x < 0 || y < 0 || x >= p.W || y >= p.H {
		return
	}
	i := y*p.W + x
	f := float32(a)
	d := p.Pix[i]
	p.Pix[i] = RGB{min(d.R+c.R*f, 255), min(d.G+c.G*f, 255), min(d.B+c.B*f, 255)}
}

// Box returns inclusive pixel bounds covering [x0,x1]×[y0,y1], clipped to the
// canvas.
func (p *Pixels) Box(x0, y0, x1, y1 float64) (int, int, int, int) {
	ix0 := max(int(math.Floor(x0)), 0)
	iy0 := max(int(math.Floor(y0)), 0)
	ix1 := min(int(math.Ceil(x1)), p.W-1)
	iy1 := min(int(math.Ceil(y1)), p.H-1)
	return ix0, iy0, ix1, iy1
}

// Coverage maps a signed distance (negative inside) to antialiased opacity.
func Coverage(d float64) float64 { return Clamp01(0.5 - d) }

// Fill paints every pixel with c; BG is unchanged.
func (p *Pixels) Fill(c RGB) {
	for i := range p.Pix {
		p.Pix[i] = c
	}
}

// Rect fills a rectangle; fractional edges are antialiased by coverage.
func (p *Pixels) Rect(x, y, w, h float64, c RGB, a float64) {
	x0, y0, x1, y1 := p.Box(x, y, x+w, y+h)
	for py := y0; py <= y1; py++ {
		cy := Clamp01(min(float64(py)+1, y+h) - max(float64(py), y))
		for px := x0; px <= x1; px++ {
			cx := Clamp01(min(float64(px)+1, x+w) - max(float64(px), x))
			p.Blend(px, py, c, a*cx*cy)
		}
	}
}

// VGradient fills rows y0..y1 with a vertical gradient from top to bottom.
func (p *Pixels) VGradient(y0, y1 int, top, bottom RGB) {
	for y := max(y0, 0); y <= min(y1, p.H-1); y++ {
		c := Mix(top, bottom, float64(y-y0)/max(float64(y1-y0), 1))
		for x := 0; x < p.W; x++ {
			p.Pix[y*p.W+x] = c
		}
	}
}

// Disc fills an antialiased circle.
func (p *Pixels) Disc(cx, cy, r float64, c RGB, a float64) {
	p.RoundRect(cx-r, cy-r, 2*r, 2*r, r, 0, c, a)
}

// Glow adds soft light around (cx, cy), fading to zero at radius r.
func (p *Pixels) Glow(cx, cy, r float64, c RGB, a float64) {
	x0, y0, x1, y1 := p.Box(cx-r, cy-r, cx+r, cy+r)
	for py := y0; py <= y1; py++ {
		for px := x0; px <= x1; px++ {
			d := math.Hypot(float64(px)+0.5-cx, float64(py)+0.5-cy) / r
			if d < 1 {
				f := 1 - d
				p.Add(px, py, c, a*f*f)
			}
		}
	}
}

// Arc strokes a ring of radius r around (cx, cy) from angle a0 to a1 (radians,
// 0 = 12 o'clock, clockwise) with round caps; a0=0, a1=2π is a full ring.
func (p *Pixels) Arc(cx, cy, r, thick, a0, a1 float64, c RGB, alpha float64) {
	if a1 <= a0 {
		return
	}
	half := thick / 2
	if thick > 0 && a1-a0 >= 2*math.Pi-1e-9 {
		// A full ring is a RoundRect outline with the same distance field.
		outer := r + half
		p.RoundRect(cx-outer, cy-outer, 2*outer, 2*outer, outer, thick, c, alpha)
		return
	}
	capA := [2][2]float64{
		{cx + r*math.Sin(a0), cy - r*math.Cos(a0)},
		{cx + r*math.Sin(a1), cy - r*math.Cos(a1)},
	}
	x0, y0, x1, y1 := p.Box(cx-r-half-1, cy-r-half-1, cx+r+half+1, cy+r+half+1)
	for py := y0; py <= y1; py++ {
		for px := x0; px <= x1; px++ {
			fx, fy := float64(px)+0.5-cx, float64(py)+0.5-cy
			d := math.Abs(math.Hypot(fx, fy)-r) - half
			if d >= 0.5 {
				continue // the caps sit on the ring, so they can't reach it either
			}
			ang := math.Atan2(fx, -fy)
			if ang < 0 {
				ang += 2 * math.Pi
			}
			if !angleIn(ang, a0, a1) {
				d = min(
					math.Hypot(float64(px)+0.5-capA[0][0], float64(py)+0.5-capA[0][1]),
					math.Hypot(float64(px)+0.5-capA[1][0], float64(py)+0.5-capA[1][1]),
				) - half
			}
			p.Blend(px, py, c, alpha*Coverage(d))
		}
	}
}

func angleIn(a, a0, a1 float64) bool {
	rel := math.Mod(a-a0, 2*math.Pi)
	if rel < 0 {
		rel += 2 * math.Pi
	}
	return rel <= a1-a0
}

// Line strokes an antialiased line with round caps.
func (p *Pixels) Line(xa, ya, xb, yb, width float64, c RGB, a float64) {
	half := width / 2
	x0, y0, x1, y1 := p.Box(min(xa, xb)-half-1, min(ya, yb)-half-1, max(xa, xb)+half+1, max(ya, yb)+half+1)
	dx, dy := xb-xa, yb-ya
	l2 := dx*dx + dy*dy
	reach := (half + 0.5) * (half + 0.5) // beyond this distance², coverage is 0
	for py := y0; py <= y1; py++ {
		for px := x0; px <= x1; px++ {
			fx, fy := float64(px)+0.5, float64(py)+0.5
			t := 0.0
			if l2 > 0 {
				t = Clamp01(((fx-xa)*dx + (fy-ya)*dy) / l2)
			}
			ex, ey := fx-(xa+t*dx), fy-(ya+t*dy)
			d2 := ex*ex + ey*ey
			if d2 >= reach {
				continue // most of a diagonal line's bounding box
			}
			p.Blend(px, py, c, a*Coverage(math.Sqrt(d2)-half))
		}
	}
}

// RoundRect fills a rounded rectangle, or only an outline of thickness stroke
// if > 0.
func (p *Pixels) RoundRect(x, y, w, h, radius, stroke float64, c RGB, a float64) {
	radius = min(radius, w/2, h/2)
	x0, y0, x1, y1 := p.Box(x-1, y-1, x+w+1, y+h+1)
	cx, cy := x+w/2, y+h/2
	hx, hy := w/2-radius, h/2-radius
	for py := y0; py <= y1; py++ {
		qy := math.Abs(float64(py)+0.5-cy) - hy
		for px := x0; px <= x1; px++ {
			qx := math.Abs(float64(px)+0.5-cx) - hx
			// Signed distance; sqrt only in corners. Plain comparisons:
			// builtin max/min's NaN handling costs ~30% here.
			ox, oy := qx, qy
			if ox < 0 {
				ox = 0
			}
			if oy < 0 {
				oy = 0
			}
			in := qx
			if qy > in {
				in = qy
			}
			if in > 0 {
				in = 0
			}
			var d float64
			if ox == 0 && oy == 0 {
				d = in - radius
			} else {
				d = math.Sqrt(ox*ox+oy*oy) + in - radius
			}
			if stroke > 0 {
				d = math.Abs(d+stroke/2) - stroke/2
			} else if d <= -0.5 {
				p.Blend(px, py, c, a)
				continue
			}
			p.Blend(px, py, c, a*Coverage(d))
		}
	}
}
