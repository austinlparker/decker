package decker

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

var stockBlockFonts = []*FigletFont{BlockShadow, BlockSolid, BlockSmall, BlockHuge, BlockFancy}

func TestFigletFontsLoadAndRender(t *testing.T) {
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

func TestBundledFigletFontCatalog(t *testing.T) {
	want := []string{
		"Decker Geometric", "Decker Sign", "Decker Slab", "Decker Square", "Decker Stencil",
		"Spleen 12x24", "Spleen 5x8", "Spleen 6x12", "Spleen 6x12 Shadow", "Spleen 8x16",
	}
	names := StockFigletFontNames()
	if !slices.Equal(names, want) {
		t.Fatalf("font catalog = %v, want %v", names, want)
	}
	names[0] = "changed by caller"
	if !slices.Equal(StockFigletFontNames(), want) {
		t.Fatal("caller's mutation changed the catalog")
	}
	for _, name := range want {
		t.Run(name, func(t *testing.T) {
			f := StockFigletFont(name)
			if f.Name != name || f.Width("A A") <= f.Width("AA") {
				t.Fatal("font name or spacing is incorrect")
			}
			for r := rune('!'); r <= '~'; r++ {
				if !f.Has(string(r)) {
					t.Errorf("missing glyph %q", r)
					continue
				}
				p := NewPixels(f.Width(string(r))*2+8, f.Height*4+8, RGB{})
				Block{Font: f, Scale: 2, Color: RGB{255, 255, 255}}.Draw(p, string(r), 2, 2)
				if !slices.ContainsFunc(p.Pix, func(px RGB) bool { return px != (RGB{}) }) {
					t.Errorf("glyph %q has no paintable strokes", r)
				}
			}
		})
	}
}

func TestConsumerFigletFont(t *testing.T) {
	data := []byte("flf2a$ 1 1 3 -1 0\n$@@\n" + strings.Repeat("█@@\n", 94))
	parsed, err := ParseFigletFont("Consumer", data)
	if err != nil {
		t.Fatal(err)
	}
	loaded := LoadFigletFont(fstest.MapFS{"custom/Consumer.flf": {Data: data}}, "custom/Consumer.flf")
	for _, f := range []*FigletFont{parsed, loaded} {
		if f.Name != "Consumer" {
			t.Fatalf("font name = %q", f.Name)
		}
		chosen, lines, scale := FitBlock("A A", 60, 20, 1, 0, f)
		if chosen != f || len(lines) != 1 || lines[0] != "A A" {
			t.Fatalf("FitBlock did not preserve the consumer's font and text: %v %v", chosen, lines)
		}
		p := NewPixels(60, 20, RGB{})
		Block{Font: chosen, Scale: scale, Color: RGB{255, 255, 255}}.Draw(p, lines[0], 0, 0)
		if p.At(0, 0) == (RGB{}) || p.At(int(scale), 0) != (RGB{}) || p.At(int(2*scale), 0) == (RGB{}) {
			t.Fatal("consumer glyphs or their space were not drawn correctly")
		}
	}
}

func TestParseFigletFontErrors(t *testing.T) {
	for _, data := range []string{
		"", "not a font", "flf2a 1 1 3 -1 0", "flf2a$$ 1 1 3 -1 0",
		"flf2a$ 0 1 3 -1 0", "flf2a$ -1 1 3 -1 0", "flf2a$ nope 1 3 -1 0",
		"flf2a$ 999999999 1 3 -1 0", "flf2a$ 1 1 3 nope 0",
		"flf2a$ 1 1 3 -1 -1", "flf2a$ 1 1 3 -1 nope", "flf2a$ 1 1 3 -1 1",
		"flf2a$ 1 1 3 -1 0\n█@@\n",
	} {
		t.Run(data, func(t *testing.T) {
			if font, err := ParseFigletFont("broken", []byte(data)); err == nil || font != nil {
				t.Fatalf("ParseFigletFont = %v, %v; want nil and an error", font, err)
			}
		})
	}
}

func TestStockFigletFontsDrawPrintableASCII(t *testing.T) {
	for _, f := range stockBlockFonts {
		t.Run(f.Name, func(t *testing.T) {
			if f.Width("A A") <= f.Width("AA") {
				t.Fatal("spaces must advance the text")
			}
			if reflect.DeepEqual(f.glyph('A'), f.glyph('a')) {
				t.Fatal("stock fonts must preserve distinct lowercase glyphs")
			}
			for r := rune('!'); r <= '~'; r++ {
				if !f.Has(string(r)) {
					t.Errorf("missing printable ASCII glyph %q", r)
					continue
				}
				p := NewPixels(64, 64, RGB{})
				Block{Font: f, Scale: 2, Color: RGB{255, 255, 255}}.Draw(p, string(r), 0, 0)
				ink := false
				for _, px := range p.Pix {
					if px != (RGB{}) {
						ink = true
						break
					}
				}
				if !ink {
					t.Errorf("glyph %q has no paintable strokes", r)
				}
			}
		})
	}
}

func TestBlockShadowHasShading(t *testing.T) {
	shade := false
	for _, row := range BlockShadow.render("AgjM") {
		for _, cell := range row {
			shade = shade || cell.r == '░'
		}
	}
	if !shade {
		t.Fatal("the shadow font must include shaded cells")
	}
}

func TestFigHasAndDropQuotes(t *testing.T) {
	if got := BlockSolid.DropQuotes("aren't"); got != "aren't" {
		t.Errorf("DropQuotes = %q, want aren't", got)
	}
	if BlockSolid.Has("é") {
		t.Error("stock FIGlet conversions load ASCII only")
	}
	if !BlockSolid.Has("Hello 2025\nok") {
		t.Error("Spleen should have letters and digits")
	}
}

func TestFigWrapKeepsBreaks(t *testing.T) {
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
