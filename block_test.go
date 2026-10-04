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
