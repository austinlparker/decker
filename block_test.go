package decker

import (
	"strings"
	"testing"
)

func TestFitBlockFitsTheBox(t *testing.T) {
	const maxW, maxH = 300.0, 120.0
	f, lines, scale := FitBlock("THE DOOM LOOP", maxW, maxH, 2, 1, BlockHuge, BlockShadow)
	if len(lines) > 2 {
		t.Fatalf("got %d lines, want at most 2", len(lines))
	}
	w, h := Block{Font: f, Scale: scale, Gap: 1}.Size(strings.Join(lines, "\n"))
	if w > maxW || h > maxH {
		t.Errorf("%s at scale %.1f is %.0f×%.0f px, want within %.0f×%.0f", f.Name, scale, w, h, maxW, maxH)
	}
	if scale < 1 {
		t.Errorf("scale %.1f: letters should fill a box this big", scale)
	}
}

// At the smallest scale, a wrap that fits must beat a longer one that
// doesn't: snapScale lifts both to 0.5, and fewer lines used to win the tie,
// running the text off the screen when a wrap that fit was right there.
func TestFitBlockWrapsRatherThanOverflowsAtSmallestScale(t *testing.T) {
	const s = "Better MCPs through fast feedback"
	f := BlockSolid
	two := f.widest(f.Wrap(s, f.Width(s)*6/10))
	maxW := float64(two)*0.5 + 1 // two lines fit at 0.5; one line needs about 0.27
	const maxH = 1000.0
	if one := float64(f.Width(s)) * 0.5; one <= maxW {
		t.Fatalf("one line is %.0f px at 0.5, want wider than the %.0f px box for this test", one, maxW)
	}
	_, lines, scale := FitBlock(s, maxW, maxH, 2, 1, f)
	if w, h := (Block{Font: f, Scale: scale, Gap: 1}).Size(strings.Join(lines, "\n")); w > maxW || h > maxH {
		t.Errorf("%q at scale %.1f is %.0f×%.0f px, want within %.0f×%.0f", lines, scale, w, h, maxW, maxH)
	}
}

// When no wrap fits even at the smallest scale, FitBlock still picks the one
// that overflows least.
func TestFitBlockOverflowsLeastWhenNothingFits(t *testing.T) {
	_, lines, scale := FitBlock("Better MCPs through fast feedback", 10, 1000, 2, 1, BlockSolid)
	if scale != 0.5 || len(lines) != 2 {
		t.Errorf("got %q at scale %.1f, want two lines at 0.5", lines, scale)
	}
}

// A preferred font that only gets its size by overflowing must lose to a
// later one that fits.
func TestFitBlockPrefersAFontThatFits(t *testing.T) {
	const s = "FEEDBACK"
	small, huge := BlockSmall, BlockHuge
	maxW := float64(small.Width(s))*0.5 + 1
	if float64(huge.Width(s))*0.5 <= maxW {
		t.Fatalf("%s is no wider than %s for this test", huge.Name, small.Name)
	}
	f, lines, scale := FitBlock(s, maxW, 1000, 1, 0, huge, small)
	if w, _ := (Block{Font: f, Scale: scale}).Size(strings.Join(lines, "\n")); w > maxW {
		t.Errorf("%s at scale %.1f is %.0f px wide, want within %.0f", f.Name, scale, w, maxW)
	}
}

func TestFitBlockResultIsACopy(t *testing.T) {
	_, a, _ := FitBlock("ONE TWO", 400, 100, 2, 1, BlockSolid)
	a[0] = "scribble"
	_, b, _ := FitBlock("ONE TWO", 400, 100, 2, 1, BlockSolid)
	if b[0] == "scribble" {
		t.Error("FitBlock handed out its cached slice")
	}
}

func TestFigWrapBalances(t *testing.T) {
	lines := BlockShadow.Wrap("What Your MCP Server Does", BlockShadow.Width("What Your MCP Server"))
	if len(lines) != 2 || len(strings.Fields(lines[0])) != 3 || len(strings.Fields(lines[1])) != 2 {
		t.Errorf("got %q, want What Your MCP / Server Does", lines)
	}
}

func TestBlockChainMerges(t *testing.T) {
	red, blue := RGB{255, 0, 0}, RGB{0, 0, 255}
	fx := BlockChain(
		func(BlockCell) BlockFX { return BlockFX{DX: 1, Alpha: 0.5, Bright: 0.2, Color: &red, Rune: 'a'} },
		func(BlockCell) BlockFX { return BlockFX{DX: 2, DY: 1, Alpha: 0.5, Bright: 0.6, Color: &blue} },
	)(BlockCell{})
	if fx.DX != 3 || fx.DY != 1 || fx.Alpha != 0.25 || fx.Bright != 0.6 || fx.Rune != 'a' || *fx.Color != blue {
		t.Errorf("BlockChain = %+v", fx)
	}
}

func TestBlockDrawsAndHidesCells(t *testing.T) {
	lit := func(p *Pixels) (n int) {
		for _, px := range p.Pix {
			if px != (RGB{}) {
				n++
			}
		}
		return
	}
	b := Block{Font: BlockSolid, Scale: 2, Color: RGB{255, 255, 255}}
	p := NewPixels(120, 60, RGB{})
	b.Draw(p, "Hi", 5, 5)
	if lit(p) == 0 {
		t.Fatal("Block.Draw changed no pixels")
	}
	b.FX = func(BlockCell) BlockFX { return BlockFX{} }
	p = NewPixels(120, 60, RGB{})
	b.Draw(p, "Hi", 5, 5)
	if lit(p) != 0 {
		t.Error("zero BlockFX should hide every cell")
	}
}
