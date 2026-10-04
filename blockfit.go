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
	f     *FigFont
	lines []string
	scale float64
}

// Slides call FitBlock every frame with the same arguments.
var fitBlocks = memo[fitBlockKey, fitBlockResult]{max: 4096}

// FitBlock picks a font and scale at which s, wrapped to at most maxLines
// lines with gap blank rows between them, is as big as possible inside a
// maxW×maxH pixel box. Fonts missing a character of s are skipped. Fonts
// are listed in order of preference: a later font only wins if its letters
// come out clearly (15%+) taller. Scales snap to half pixels (whole pixels
// from 4 up), for nearly crisp edges.
func FitBlock(s string, maxW, maxH float64, maxLines, gap int, fonts ...*FigFont) (*FigFont, []string, float64) {
	var ids strings.Builder
	for _, f := range fonts {
		fmt.Fprintf(&ids, "%p,", f)
	}
	r := fitBlocks.get(fitBlockKey{s, maxW, maxH, maxLines, gap, ids.String()}, func() fitBlockResult {
		return fitBlock(s, maxW, maxH, maxLines, gap, fonts)
	})
	return r.f, slices.Clone(r.lines), r.scale
}

func fitBlock(s string, maxW, maxH float64, maxLines, gap int, fonts []*FigFont) fitBlockResult {
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
func fitFont(f *FigFont, text string, maxW, maxH float64, maxLines, gap int) (best []string, bestScale float64) {
	bestWidest := 0
	// Try narrower and narrower wraps; keep the one that scales up most.
	for tw := f.widest(strings.Split(text, "\n")); tw > 0; tw = tw * 94 / 100 {
		lines := balancedWrap(f, text, tw)
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

// balancedWrap wraps each paragraph of s to at most maxW cells in as few
// lines as greedy wrapping needs, but with the breaks evened out: it finds
// the narrowest width that still takes that many lines. "What Your MCP
// Server / Does" becomes "What Your MCP / Server Does".
func balancedWrap(f *FigFont, s string, maxW int) []string {
	var out []string
	for _, para := range strings.Split(s, "\n") {
		n := len(f.Wrap(para, maxW))
		lo, hi := 1, maxW // narrowest width that keeps n lines is in [lo, hi]
		for lo < hi {
			mid := (lo + hi) / 2
			if len(f.Wrap(para, mid)) <= n {
				hi = mid
			} else {
				lo = mid + 1
			}
		}
		out = append(out, f.Wrap(para, hi)...)
	}
	return out
}

// snapScale rounds a scale down to half pixels (whole pixels from 4 up), so
// block edges land on or halfway between pixels and stay nearly crisp.
func snapScale(s float64) float64 {
	if s >= 4 {
		return math.Floor(s)
	}
	return math.Max(math.Floor(s*2)/2, 0.5)
}
