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
	return slices.Clone(wraps.get(wrapKey{f, s, size, maxW}, func() []string {
		return wrapBalanced(s, maxW, func(l string) float64 { return f.Measure(l, size) })
	}))
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

type fitKey struct {
	f          *Font
	text       string // text, or length-prefixed FitAll parts
	maxW, maxH float64
	maxSize    int
	leading    float64
	all        bool
}

type fitResult struct {
	size  int
	lines []string
}

var fitted = memo[fitKey, fitResult]{max: 5000}

// fit finds the largest size at which parts, each wrapped to maxW, stack into
// at most maxH. If none fits it returns the smallest size wrapped to the width:
// running taller beats running off the side of the screen.
func (f *Font) fit(parts []string, maxW, maxH float64, maxSize int, leading float64) (int, []string) {
	wrap := func(size int) (lines []string) {
		for _, part := range parts {
			lines = append(lines, f.Wrap(part, size, maxW)...)
		}
		return lines
	}
	for size := maxSize; size > minFitSize; size-- {
		lines := wrap(size)
		if fits(lines, maxW, func(l string) float64 { return f.Measure(l, size) }) && float64(len(lines))*float64(size)*leading <= maxH {
			return size, lines
		}
	}
	return minFitSize, wrap(minFitSize)
}

// Fit returns the largest size (at most maxSize) at which s, wrapped to maxW,
// fits maxW×maxH, with the wrapped text. A zero leading means DefaultLeading.
func (f *Font) Fit(s string, maxW, maxH float64, maxSize int, leading float64) (int, string) {
	leading = leadingOr(leading)
	r := fitted.get(fitKey{f, s, maxW, maxH, maxSize, leading, false}, func() fitResult {
		size, lines := f.fit([]string{s}, maxW, maxH, maxSize, leading)
		return fitResult{size, lines}
	})
	return r.size, strings.Join(r.lines, "\n")
}

// FitAll is Fit for several parts stacked at DefaultLeading; it returns the
// size and all the wrapped lines in order.
func FitAll(f *Font, parts []string, maxW, maxH float64, maxSize int) (int, []string) {
	r := fitted.get(fitKey{f, labelsKey(parts), maxW, maxH, maxSize, DefaultLeading, true}, func() fitResult {
		size, lines := f.fit(parts, maxW, maxH, maxSize, DefaultLeading)
		return fitResult{size, lines}
	})
	return r.size, slices.Clone(r.lines)
}
