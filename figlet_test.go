package decker

import (
	"fmt"
	"strings"
	"testing"
)

func TestFigFontsLoadAndRender(t *testing.T) {
	for _, f := range []*FigFont{BlockShadow, BlockSolid, BlockSmall, BlockHuge, BlockFancy} {
		rows, owner := f.render("Hi!")
		if len(rows) == 0 || len(rows) != len(owner) {
			t.Fatalf("%s: rendered %d rows", f.Name, len(rows))
		}
		ink := 0
		for y, r := range rows {
			if len(r) != len(owner[y]) {
				t.Fatalf("%s: row %d has %d cells but %d owners", f.Name, y, len(r), len(owner[y]))
			}
			ink += len(strings.TrimSpace(string(r)))
		}
		if ink == 0 {
			t.Errorf("%s rendered nothing", f.Name)
		}
		if strings.ContainsRune(fmt.Sprint(rows), '\u00a0') {
			t.Errorf("%s left hard blanks in the output", f.Name)
		}
	}
}

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

// TestPrintFontSamples shows each font's size; run with -v to eyeball them.
func TestPrintFontSamples(t *testing.T) {
	if !testing.Verbose() {
		t.Skip("run with -v to print samples")
	}
	for _, f := range []*FigFont{BlockShadow, BlockSolid, BlockSmall, BlockHuge, BlockFancy} {
		rows, _ := f.render("Doom 40%")
		fmt.Printf("== %s: %d rows, 'Doom 40%%' is %d cells wide\n", f.Name, len(rows), f.Width("Doom 40%"))
		for _, r := range rows {
			fmt.Println(string(r))
		}
	}
}
