package decker

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

type fitBlockKey struct {
	s          string
	maxW, maxH float64
	maxLines   int
	gap        int
	fonts      string // the fonts' identities, in order
}

type fitBlockResult struct {
	f     *FigletFont
	lines []string
	scale float64
}

var fitBlocks = memo[fitBlockKey, fitBlockResult]{max: 4096}

// FitBlock picks a font and scale at which s, wrapped to at most maxLines lines
// with gap blank rows between them, is as big as possible in a maxW×maxH pixel
// box. Fonts missing a character of s are skipped. Fonts are in order of
// preference: a later one only wins if its letters come out 15%+ taller, and
// a font that fits always beats one that doesn't. Scales snap to half pixels
// (whole from 4 up) for nearly crisp edges, and go no smaller than 0.5: when
// nothing fits even then, FitBlock returns the wrap that overflows least.
func FitBlock(s string, maxW, maxH float64, maxLines, gap int, fonts ...*FigletFont) (*FigletFont, []string, float64) {
	var ids strings.Builder
	for _, f := range fonts {
		fmt.Fprintf(&ids, "%p,", f)
	}
	r := fitBlocks.get(fitBlockKey{s, maxW, maxH, maxLines, gap, ids.String()}, func() fitBlockResult {
		return fitBlock(s, maxW, maxH, maxLines, gap, fonts)
	})
	return r.f, slices.Clone(r.lines), r.scale
}

func fitBlock(s string, maxW, maxH float64, maxLines, gap int, fonts []*FigletFont) fitBlockResult {
	var cands, overflowing []fitBlockResult
	for _, f := range fonts {
		text := f.DropQuotes(s)
		if !f.Has(text) {
			continue
		}
		lines, scale, fits := fitFont(f, text, maxW, maxH, maxLines, gap)
		switch {
		case lines == nil:
		case fits:
			cands = append(cands, fitBlockResult{f, lines, scale})
		default:
			overflowing = append(overflowing, fitBlockResult{f, lines, scale})
		}
	}
	// A font that fits beats one whose letters only come out taller by
	// running out of the box.
	if len(cands) == 0 {
		cands = overflowing
	}
	if len(cands) == 0 {
		f := fonts[len(fonts)-1]
		return fitBlockResult{f, f.Wrap(f.DropQuotes(s), max(int(maxW), 1)), 0.5}
	}
	letter := func(c fitBlockResult) float64 { return c.scale * 2 * float64(c.f.Rows()) }
	top := 0.0
	for _, c := range cands {
		top = math.Max(top, letter(c))
	}
	for _, c := range cands {
		if letter(c) >= top/1.15 {
			return c
		}
	}
	return cands[0]
}

// fitFont finds the wrap of text in f that scales up most inside maxW×maxH,
// or nil if even one line is more than maxLines. fits is false when no wrap
// fits even at snapScale's floor; best is then the one that overflows least.
func fitFont(f *FigletFont, text string, maxW, maxH float64, maxLines, gap int) (best []string, bestScale float64, fits bool) {
	bestWidest, bestRaw := 0, 0.0
	for tw := f.widest(strings.Split(text, "\n")); tw > 0; tw = tw * 94 / 100 {
		lines := f.Wrap(text, tw)
		if maxLines > 0 && len(lines) > maxLines {
			break
		}
		widest := f.widest(lines)
		rows := len(lines)*f.Rows() + (len(lines)-1)*gap
		raw := math.Min(maxW/float64(widest), maxH/(2*float64(rows)))
		sc := snapScale(raw)
		// snapScale's floor lifts wraps that don't fit to the same scale as
		// ones that just do, so at the same scale one that fits wins, and of
		// two that don't, the one that overflows least. Of two that fit,
		// prefer fewer lines, then the narrower one.
		ok := raw >= sc
		better := sc > bestScale
		if sc == bestScale {
			switch {
			case ok != fits:
				better = ok
			case !ok:
				better = raw > bestRaw
			default:
				better = len(lines) < len(best) || len(lines) == len(best) && widest < bestWidest
			}
		}
		if better {
			best, bestScale, bestWidest, bestRaw, fits = lines, sc, widest, raw, ok
		}
		if tw < 8 {
			break
		}
	}
	return best, bestScale, fits
}

// snapScale rounds down to half pixels (whole from 4 up) so edges stay nearly
// crisp.
func snapScale(s float64) float64 {
	if s >= 4 {
		return math.Floor(s)
	}
	return math.Max(math.Floor(s*2)/2, 0.5)
}
