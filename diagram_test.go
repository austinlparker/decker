package decker

import "testing"

func diagramCtx(step int) Ctx {
	return Ctx{W: 160, H: 40, Step: step, T: 5, StepT: 5, Theme: testTheme}
}

func inkIn(p *Pixels, r Rect) bool {
	for y := int(r.Y); y < int(r.Bottom()); y++ {
		for x := int(r.X); x < int(r.Right()); x++ {
			if p.At(x, y) != testTheme.Background {
				return true
			}
		}
	}
	return false
}

func TestTimelineRevealsByStep(t *testing.T) {
	tl := Timeline{FirstStep: 1, Items: []TimelineItem{{"Alpha", "first"}, {"Beta", "second"}, {"Gamma", "third"}}}
	r := Rect{10, 5, 140, 70}
	col := func(i int) Rect { return Rect{r.X + float64(i)*r.W/3, r.Y, r.W / 3, r.H} }
	for step := 0; step <= 4; step++ {
		p := NewPixels(160, 80, testTheme.Background)
		tl.Draw(diagramCtx(step), p, r)
		for i := 0; i < 3; i++ {
			if got, want := inkIn(p, col(i)), step >= 1+i; got != want {
				t.Errorf("step %d: item %d drawn = %v, want %v", step, i, got, want)
			}
		}
	}
}

func TestTimelineCurrentIsAccent(t *testing.T) {
	tl := Timeline{Items: []TimelineItem{{"Alpha", ""}, {"Beta", ""}}}
	r := Rect{10, 5, 140, 70}
	has := func(p *Pixels, c RGB, x0, x1 int) bool {
		for y := 0; y < p.H; y++ {
			for x := x0; x < x1; x++ {
				if p.At(x, y) == c {
					return true
				}
			}
		}
		return false
	}
	p := NewPixels(160, 80, testTheme.Background)
	tl.Draw(diagramCtx(1), p, r)
	if !has(p, testTheme.Accent, 85, 160) {
		t.Error("the current item should be in Accent")
	}
	if has(p, testTheme.Accent, 0, 80) {
		t.Error("a past item should not be in Accent")
	}
	if !has(p, testTheme.Muted, 0, 80) {
		t.Error("a past item should be in Muted")
	}
}

func TestTimelineSizeAndFit(t *testing.T) {
	items := []TimelineItem{{"Kickoff", "We agree on scope and owners"}, {"Prototype", "A rough version people can try"}, {"Launch", "Everyone gets it"}}
	r := Rect{10, 5, 140, 70}
	w, h := Timeline{Items: items}.Draw(diagramCtx(9), NewPixels(160, 80, testTheme.Background), r)
	if w != r.W || h <= 0 || h > r.H*1.25 {
		t.Errorf("horizontal size = %v x %v in %v", w, h, r)
	}
	v := Rect{10, 5, 140, 70}
	w, h = Timeline{Items: items, Vertical: true}.Draw(diagramCtx(9), NewPixels(160, 80, testTheme.Background), v)
	if h <= 0 || h > v.H*1.25 || w <= 0 || w > v.W {
		t.Errorf("vertical size = %v x %v in %v", w, h, v)
	}
	if w, h := (Timeline{}).Draw(diagramCtx(9), NewPixels(160, 80, testTheme.Background), r); w != 0 || h != 0 {
		t.Errorf("empty timeline size = %v x %v", w, h)
	}
	// Text stays inside the canvas for long labels in a narrow rect.
	long := []TimelineItem{{"A very long milestone name that wraps", "and a long detail that wraps over several lines too"}, {"B", ""}}
	Timeline{Items: long}.Draw(diagramCtx(9), NewPixels(160, 80, testTheme.Background), Rect{0, 0, 60, 70})
	Timeline{Items: long, Vertical: true}.Draw(diagramCtx(9), NewPixels(160, 80, testTheme.Background), Rect{0, 0, 60, 70})
}

func TestProcessRevealsByStep(t *testing.T) {
	pr := Process{FirstStep: 2, Steps: []string{"Plan", "Build", "Ship"}}
	r := Rect{10, 5, 140, 40}
	for step := 0; step <= 5; step++ {
		p := NewPixels(160, 80, testTheme.Background)
		w, h := pr.Draw(diagramCtx(step), p, r)
		if w != r.W || h <= 0 || h > r.H {
			t.Errorf("size = %v x %v in %v", w, h, r)
		}
		for i := 0; i < 3; i++ {
			chev := Rect{r.X + float64(i)*r.W/3 + 5, r.Y, r.W/3 - 10, h}
			if got, want := inkIn(p, chev), step >= 2+i; got != want {
				t.Errorf("step %d: chevron %d drawn = %v, want %v", step, i, got, want)
			}
		}
	}
}

func TestProcessActiveIsFilled(t *testing.T) {
	pr := Process{Steps: []string{"Plan", "Build"}}
	p := NewPixels(160, 80, testTheme.Background)
	pr.Draw(diagramCtx(1), p, Rect{10, 5, 140, 40})
	if got := p.At(100, 10); got != testTheme.Accent {
		t.Errorf("active chevron fill = %v, want Accent", got)
	}
	if got := p.At(20, 10); got == testTheme.Accent {
		t.Error("a finished chevron should not be filled in Accent")
	}
	if w, h := (Process{}).Draw(diagramCtx(1), p, Rect{}); w != 0 || h != 0 {
		t.Errorf("empty process size = %v x %v", w, h)
	}
	Process{Steps: []string{"A single step with a long name"}}.Draw(diagramCtx(1), p, Rect{10, 5, 140, 40})
}
