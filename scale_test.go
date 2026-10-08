package decker

import (
	"slices"
	"testing"
)

func TestNiceScaleIsTheChartsAxis(t *testing.T) {
	for _, tc := range []struct {
		lo, hi        float64
		ticks         int
		min, max, stp float64
	}{
		{0, 347, 5, 0, 400, 100},
		{347, 0, 5, 0, 400, 100}, // either order
		{0, 0.93, 6, 0, 1, 0.2},
		{-12, 30, 5, -20, 30, 10},
		{5, 5, 5, 2, 8, 1}, // one value, widened around it
	} {
		s := NiceScale(tc.lo, tc.hi, tc.ticks, 0, 100)
		if s.Min != tc.min || s.Max != tc.max || s.Step != tc.stp {
			t.Errorf("NiceScale(%v, %v, %d) = %v..%v by %v, want %v..%v by %v", tc.lo, tc.hi, tc.ticks, s.Min, s.Max, s.Step, tc.min, tc.max, tc.stp)
		}
		if a := niceScale(min(tc.lo, tc.hi), max(tc.lo, tc.hi), tc.ticks); s.Min != a.lo || s.Max != a.hi || s.Step != a.step {
			t.Errorf("NiceScale(%v, %v) differs from the charts' axis %+v", tc.lo, tc.hi, a)
		}
	}
}

func TestScaleAt(t *testing.T) {
	s := Scale{Min: 0, Max: 400, Step: 100, From: 100, To: 500}
	for v, want := range map[float64]float64{0: 100, 400: 500, 200: 300, 347: 447, -100: 0, 500: 600} {
		if got := s.At(v); got != want {
			t.Errorf("At(%v) = %v, want %v", v, got, want)
		}
	}
	up := Scale{Min: 0, Max: 10, From: 200, To: 100} // a y axis
	if got := up.At(2.5); got != 175 {
		t.Errorf("upward At(2.5) = %v, want 175", got)
	}
	if got := (Scale{Min: 3, Max: 3, From: 7, To: 9}).At(3); got != 7 {
		t.Errorf("no range: At = %v, want From", got)
	}
}

func TestScaleTicksAndLabels(t *testing.T) {
	s := NiceScale(0, 418, 5, 0, 1)
	s.Max = 418 // the data's own end: ticks stay on the step
	if got := s.Ticks(); !slices.Equal(got, []float64{0, 100, 200, 300, 400}) {
		t.Errorf("Ticks = %v", got)
	}
	var labels []string
	for _, v := range NiceScale(0, 1, 5, 0, 1).Ticks() {
		labels = append(labels, s.Label(v))
	}
	if !slices.Equal(labels, []string{"0", "0.2", "0.4", "0.6", "0.8", "1"}) {
		t.Errorf("labels = %v", labels)
	}
	if got := s.Label(12500); got != "12.5k" {
		t.Errorf("Label(12500) = %q, want the charts' 12.5k", got)
	}
}
