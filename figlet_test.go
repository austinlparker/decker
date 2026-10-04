package decker

import (
	"strings"
	"testing"
)

var stockBlockFonts = []*FigFont{BlockShadow, BlockSolid, BlockSmall, BlockHuge, BlockFancy}

func TestFigFontsLoadAndRender(t *testing.T) {
	for _, f := range stockBlockFonts {
		rows := f.render("Hi!")
		if len(rows) == 0 || len(rows) > f.Rows() {
			t.Fatalf("%s: rendered %d rows, Rows() is %d", f.Name, len(rows), f.Rows())
		}
		ink := 0
		for _, row := range rows {
			for _, c := range row {
				if c.r == hardBlank {
					t.Errorf("%s left hard blanks in the output", f.Name)
				}
				if c.r != ' ' {
					ink++
					if c.owner < 0 || c.owner > 2 {
						t.Errorf("%s: cell %q owned by character %d of 3", f.Name, c.r, c.owner)
					}
				}
			}
		}
		if ink == 0 {
			t.Errorf("%s rendered nothing", f.Name)
		}
	}
}

func TestFigHasAndDropQuotes(t *testing.T) {
	if got := BlockSolid.DropQuotes("aren't"); got != "arent" {
		t.Errorf("DropQuotes = %q, want arent", got)
	}
	if BlockSolid.Has("é") {
		t.Error("ANSI Regular has no é")
	}
	if !BlockSolid.Has("Hello 2025\nok") {
		t.Error("ANSI Regular should have letters and digits")
	}
}

func TestFigWrapIsGreedy(t *testing.T) {
	const s = "one two three four\nfive six seven eight nine"
	lines := BlockSmall.Wrap(s, 30)
	for _, l := range lines {
		if w := BlockSmall.Width(l); w > 30 {
			t.Errorf("line %q is %d cells wide, want <= 30", l, w)
		}
	}
	for _, l := range lines {
		if strings.Contains(l, "four") && strings.Contains(l, "five") {
			t.Errorf("explicit line break was not kept: %q", lines)
		}
	}
}
