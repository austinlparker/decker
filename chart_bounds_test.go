package decker

import "testing"

// inkMask draws with a theme whose colors are all black on black except those
// keep sets, so the mask shows only what those colors paint. Layout does not
// depend on color, so masks of the same draw with different keeps line up.
func inkMask(w, h int, keep func(*Theme), draw func(c Ctx, p *Pixels)) []bool {
	th := *testTheme
	th.Series = nil
	th.Overlay = nil
	black := RGB{}
	th.Background, th.Text, th.Muted, th.Faint = black, black, black, black
	th.Accent, th.Accent2, th.Warn, th.Good, th.Panel = black, black, black, black, black
	keep(&th)
	c := Ctx{W: w, H: h, T: Settled, StepT: Settled, Theme: &th}
	p := NewPixels(w, 2*h, black)
	draw(c, p)
	mask := make([]bool, len(p.Pix))
	for i, px := range p.Pix {
		mask[i] = px != black
	}
	return mask
}

// touches reports a pixel inked in a that has an inked pixel of b within one
// pixel of it: two things painted on top of each other, or touching.
func touches(a, b []bool, w, h int) (x, y int, ok bool) {
	for i, on := range a {
		if !on {
			continue
		}
		px, py := i%w, i/w
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				qx, qy := px+dx, py+dy
				if qx >= 0 && qy >= 0 && qx < w && qy < h && b[qy*w+qx] {
					return px, py, true
				}
			}
		}
	}
	return 0, 0, false
}

// TestNegativeValueLabelsClearCategories draws bars at +10 and -10 and checks
// the value labels (Text) never touch the category labels (Muted), in both
// orientations.
func TestNegativeValueLabelsClearCategories(t *testing.T) {
	for _, sz := range [][2]int{{240, 67}, {682, 171}, {120, 36}} {
		for _, horiz := range []bool{false, true} {
			chart := BarChart{Labels: []string{"Negative", "Positive"}, Values: []float64{-10, 10}, ShowValues: true, Horizontal: horiz}
			draw := func(c Ctx, p *Pixels) { chart.Draw(c, p, c.Frame()) }
			values := inkMask(sz[0], sz[1], func(th *Theme) { th.Text = RGB{255, 255, 255} }, draw)
			// Muted is also the axis labels and the zero line, but none of
			// those belong beside a value label.
			cats := inkMask(sz[0], sz[1], func(th *Theme) { th.Muted = RGB{0, 255, 0} }, draw)
			if x, y, hit := touches(values, cats, sz[0], 2*sz[1]); hit {
				t.Errorf("%dx%d horizontal=%v: a value label touches a category label at %d,%d", sz[0], sz[1], horiz, x, y)
			}
		}
	}
}

// TestShapesStayInsideSmallRects checks the strokes, bars, dots and arcs of
// every chart stay within a pixel of the rect they were given, however small.
// Text is left out: it is never drawn below the smallest readable size, so it
// can't promise to fit a rect smaller than a line.
func TestShapesStayInsideSmallRects(t *testing.T) {
	bright := func(th *Theme) {
		th.Accent, th.Accent2, th.Good, th.Warn = RGB{255, 0, 0}, RGB{0, 255, 0}, RGB{0, 0, 255}, RGB{255, 255, 0}
	}
	data := [][]float64{{3, -1, 4, 1, 5}, {2, 7, 1, 8, 2}}
	labels := []string{"a", "b", "c", "d", "e"}
	for _, sz := range [][2]int{{682, 171}, {240, 67}} {
		for _, r := range []Rect{
			{20, 20, 4, 4}, {20, 20, 10, 10}, {20, 20, 30, 20}, {20, 20, 60, 40},
			{5, 5, 3, 50}, {5, 5, 50, 3}, {30, 30, 0.5, 0.5}, {100, 60, 120, 90},
		} {
			for name, draw := range map[string]func(c Ctx, p *Pixels){
				"sparkline":      func(c Ctx, p *Pixels) { Sparkline(c, p, r, data[0], c.Theme.Accent, 1) },
				"sparkline pair": func(c Ctx, p *Pixels) { Sparkline(c, p, r, []float64{1, 2}, c.Theme.Accent, 1) },
				"sparkline one":  func(c Ctx, p *Pixels) { Sparkline(c, p, r, []float64{1}, c.Theme.Accent, 1) },
				"bars": func(c Ctx, p *Pixels) {
					BarChart{Labels: labels, Series: data, Names: []string{"x", "y"}, ShowValues: true}.Draw(c, p, r)
				},
				"hbars": func(c Ctx, p *Pixels) {
					BarChart{Labels: labels, Series: data, Horizontal: true, ShowValues: true}.Draw(c, p, r)
				},
				"line":  func(c Ctx, p *Pixels) { LineChart{Labels: labels, Series: data, Points: true}.Draw(c, p, r) },
				"donut": func(c Ctx, p *Pixels) { DonutChart{Labels: labels, Values: data[1], Center: "x"}.Draw(c, p, r) },
			} {
				w, h := sz[0], sz[1]
				mask := inkMask(w, h, bright, draw)
				for i, on := range mask {
					x, y := float64(i%w)+0.5, float64(i/w)+0.5
					if on && (x < r.X-1 || x > r.Right()+1 || y < r.Y-1 || y > r.Bottom()+1) {
						t.Errorf("%dx%d %s in %+v: ink at %v,%v is outside it", w, h, name, r, x, y)
						break
					}
				}
			}
		}
	}
}
