package decker

import (
	"slices"
	"strings"
)

// wrapGreedy breaks s at spaces into lines measuring at most maxW, keeping "\n"
// breaks. A word wider than maxW gets its own line.
func wrapGreedy(s string, maxW float64, measure func(line string) float64) []string {
	var out []string
	for _, para := range strings.Split(s, "\n") {
		line := ""
		for _, word := range strings.Fields(para) {
			try := word
			if line != "" {
				try = line + " " + word
			}
			if line != "" && measure(try) > maxW {
				out = append(out, line)
				line = word
			} else {
				line = try
			}
		}
		out = append(out, line)
	}
	return out
}

type wrapKey struct {
	f    *Font
	s    string
	size int
	maxW float64
}

// Slides lay out every frame, and balancing takes ~13 greedy wraps per
// paragraph.
var wraps = memo[wrapKey, []string]{max: 8192}

// Wrap breaks s at spaces into lines no wider than maxW, keeping "\n" breaks.
// Lines are balanced: it uses the narrowest width needing no more lines than
// maxW does, so "What Your MCP / Server Does" beats "What Your MCP Server /
// Does".
func (f *Font) Wrap(s string, size int, maxW float64) []string {
	return slices.Clone(f.wrapped(s, size, maxW))
}

// wrapped is Wrap without the copy, for callers that only read the lines; the
// slice is the cache's and must not be modified.
func (f *Font) wrapped(s string, size int, maxW float64) []string {
	return wraps.get(wrapKey{f, s, size, maxW}, func() []string {
		return wrapBalanced(s, maxW, func(l string) float64 { return f.Measure(l, size) })
	})
}

// widest is the width of the widest of lines at size, 0 for none.
func (f *Font) widest(lines []string, size int) float64 {
	w := 0.0
	for _, l := range lines {
		w = max(w, f.Measure(l, size))
	}
	return w
}

// wrapBalanced is wrapGreedy per paragraph, narrowed by bisection to the
// tightest width that adds no lines. Greedy line count never falls as the
// width shrinks, so the bisection is sound.
func wrapBalanced(s string, maxW float64, measure func(line string) float64) []string {
	var out []string
	for _, para := range strings.Split(s, "\n") {
		lines := wrapGreedy(para, maxW, measure)
		best, lo, hi := lines, maxW*0.4, maxW
		for range 12 {
			mid := (lo + hi) / 2
			if b := wrapGreedy(para, mid, measure); len(b) <= len(lines) && fits(b, mid, measure) {
				best, hi = b, mid
			} else {
				lo = mid
			}
		}
		out = append(out, best...)
	}
	return out
}

// fits reports whether no line is wider than maxW; an overlong word is the
// only way greedy wrapping breaks that.
func fits(lines []string, maxW float64, measure func(line string) float64) bool {
	return !slices.ContainsFunc(lines, func(l string) bool { return measure(l) > maxW })
}

const minFitSize = 6

// largestSize counts down from from and returns the first size fits accepts,
// or floor once it gets there, accepted or not. fits is called for every size
// tried, the returned one last, so it can keep the layout it made there. A
// from at or below floor is tried once and returned as it is; callers that
// mean floor as a minimum pass max(from, floor).
func largestSize(from, floor int, fits func(size int) bool) int {
	size := from
	for !fits(size) && size > floor {
		size--
	}
	return size
}

type fitKey struct {
	f       *Font
	s       string
	w, h    float64
	size    int // the largest size wanted
	leading float64
}

// fitted remembers Text.Fit's sizes: a View fits the same text every frame.
var fitted = memo[fitKey, int]{max: 5000}
