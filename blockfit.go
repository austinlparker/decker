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
// preference: a later one only wins if its letters come out 15%+ taller. Scales
// snap to half pixels (whole from 4 up) for nearly crisp edges.
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
	var cands []fitBlockResult
	for _, f := range fonts {
		text := f.DropQuotes(s)
		if !f.Has(text) {
			continue
		}
		if lines, scale := fitFont(f, text, maxW, maxH, maxLines, gap); lines != nil {
			cands = append(cands, fitBlockResult{f, lines, scale})
		}
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
// or nil if even one line is more than maxLines.
func fitFont(f *FigletFont, text string, maxW, maxH float64, maxLines, gap int) (best []string, bestScale float64) {
	bestWidest := 0
	for tw := f.widest(strings.Split(text, "\n")); tw > 0; tw = tw * 94 / 100 {
		lines := f.Wrap(text, tw)
		if maxLines > 0 && len(lines) > maxLines {
			break
		}
		widest := f.widest(lines)
		rows := len(lines)*f.Rows() + (len(lines)-1)*gap
		sc := snapScale(math.Min(maxW/float64(widest), maxH/(2*float64(rows))))
		// At the same scale, prefer fewer lines, then the narrower one.
		if sc > bestScale || (sc == bestScale && (len(lines) < len(best) ||
			len(lines) == len(best) && widest < bestWidest)) {
			best, bestScale, bestWidest = lines, sc, widest
		}
		if tw < 8 {
			break
		}
	}
	return best, bestScale
}

// snapScale rounds down to half pixels (whole from 4 up) so edges stay nearly
// crisp.
func snapScale(s float64) float64 {
	if s >= 4 {
		return math.Floor(s)
	}
	return math.Max(math.Floor(s*2)/2, 0.5)
}
