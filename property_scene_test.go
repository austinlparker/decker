package decker

import (
	"bytes"
	"math/rand/v2"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// referenceCell models occupied terminal columns, including the second
// column of wide glyphs. Writing either column erases the previous glyph.
type referenceCell struct {
	text string
	wide bool
}

func FuzzTerminalDiff(f *testing.F) {
	f.Add(uint8(5), []byte{0, 2, 255, 0, 1, 0, 0, 255, 0, 3, 0, 128})
	rng := rand.New(rand.NewPCG(9, 10))
	for range 32 {
		p := make([]byte, 128)
		for i := range p {
			p[i] = byte(rng.Uint32())
		}
		f.Add(uint8(rng.IntN(24)), p)
	}
	f.Fuzz(func(t *testing.T, wb uint8, data []byte) {
		w := int(wb%24) + 1
		g := blankGrid(w, 1, RGB{})
		defer g.release()
		var output bytes.Buffer
		writer := &termWriter{out: &output}
		defer func() { writer.prev.release() }()
		terminal := newVterm(w, 1)
		glyphs := []string{"a", "b", "界", "語", " ", halfBlock}
		for i := 0; i+4 <= len(data) && i < 256; i += 4 {
			x := int(data[i]) % w
			ch := glyphs[int(data[i+1])%len(glyphs)]
			c := &uv.Cell{Content: ch, Width: ansi.StringWidth(ch), Style: uv.Style{
				Fg: RGB{float32(data[i+2]), 43, 211}.Color(), Bg: RGB{11, float32(data[i+3]), 53}.Color(), Attrs: data[i+1] & uv.AttrBold,
			}}
			g.setUV(x, 0, c, [3]uint8{}, [3]uint8{})
			output.Reset()
			writer.write(g.clone(), "", false)
			terminal.feed(t, output.Bytes())
			terminal.check(t, g, "generated diff")
			output.Reset()
			writer.write(g.clone(), "", false)
			if output.Len() != 0 {
				t.Fatal("unchanged frame wrote terminal bytes")
			}
		}
	})
}

func referenceWrite(row []referenceCell, x int, text string, width int) {
	if x < 0 || x >= len(row) {
		return
	}
	if width == 2 && x+1 == len(row) {
		text, width = " ", 1
	}
	for i := x; i < x+width; i++ {
		if row[i].text == "" && i > 0 {
			row[i-1] = referenceCell{text: " "}
		}
		if row[i].wide {
			row[i+1] = referenceCell{text: " "}
		}
		row[i] = referenceCell{text: " "}
	}
	row[x] = referenceCell{text: text, wide: width == 2}
	if width == 2 {
		row[x+1] = referenceCell{}
	}
}

func checkReferenceRow(t *testing.T, g *grid, want []referenceCell) {
	t.Helper()
	for x, c := range want {
		if got := g.at(x, 0); got.ch != c.text || got.wide != c.wide {
			t.Fatalf("column %d = (%q,%v), want (%q,%v)", x, got.ch, got.wide, c.text, c.wide)
		}
	}
	if got := ansi.StringWidth(g.String()); got != g.W {
		t.Fatalf("rendered width = %d, want %d", got, g.W)
	}
}

func FuzzCharacterOverwrite(f *testing.F) {
	// Adjacent wide glyphs, continuation overwrites, and revisiting a lead.
	for _, p := range [][]byte{{0, 2, 1, 0, 0, 2}, {1, 2, 0, 2}, {0, 2, 1, 2}, {0, 2, 0, 0}} {
		f.Add(uint8(4), p)
	}
	rng := rand.New(rand.NewPCG(3, 4))
	for range 64 {
		p := make([]byte, 64)
		for i := range p {
			p[i] = uint8(rng.IntN(8))
		}
		f.Add(uint8(rng.IntN(8)+1), p)
	}
	f.Fuzz(func(t *testing.T, wb uint8, program []byte) {
		w := int(wb%8) + 1
		s := NewScene(w, 1, testTheme)
		defer s.Release()
		g := blankGrid(w, 1, testTheme.Background)
		defer g.release()
		want := make([]referenceCell, w)
		for i := range want {
			want[i].text = " "
		}
		glyphs := []string{"a", "b", "界", "語"}
		for i := 0; i+2 <= len(program) && i < 256; i += 2 {
			x := int(program[i]%(uint8(w)+2)) - 1
			ch := glyphs[program[i+1]%4]
			width := ansi.StringWidth(ch)
			referenceWrite(want, x, ch, width)
			s.Cell(x, 0, ch, uv.Style{})
			if x >= 0 && x < w {
				g.setUV(x, 0, &uv.Cell{Content: ch, Width: width}, [3]uint8{}, [3]uint8{})
			}
			checkReferenceRow(t, g, want)
			actual := s.toGrid()
			checkReferenceRow(t, actual, want)
			actual.release()
		}
	})
}

func FuzzSceneTextGraphemes(f *testing.F) {
	for _, s := range []string{"", "abc", "界a", "e\u0301x", "👨‍👩‍👧‍👦x", "👍🏽 x", "⚠️x", "🇺🇸x"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, input string) {
		// Generate arbitrary strings over printable clusters, retaining spaces.
		clusters := []string{"a", " ", "界", "e\u0301", "👨‍👩‍👧‍👦", "👍🏽", "⚠️", "🇺🇸"}
		text := input
		if len(text) > 256 {
			text = text[:256]
		}
		if !utf8.ValidString(text) || strings.ContainsFunc(text, func(r rune) bool { return !unicode.IsPrint(r) && r != '\u200d' }) {
			var b strings.Builder
			for _, v := range []byte(text) {
				b.WriteString(clusters[int(v)%len(clusters)])
			}
			text = b.String()
		}
		// Text's transparent spaces and Put's opaque spaces agree on a blank
		// scene. Put uses the terminal library's grapheme layout as an oracle.
		w := max(ansi.StringWidth(text)+2, 2)
		a, b := NewScene(w, 1, testTheme), NewScene(w, 1, testTheme)
		defer a.Release()
		defer b.Release()
		a.Text(0, 0, text, nil)
		b.Put(0, 0, text)
		ga, gb := a.toGrid(), b.toGrid()
		defer ga.release()
		defer gb.release()
		for i, c := range ga.Cells {
			if c.ch != gb.Cells[i].ch || c.wide != gb.Cells[i].wide {
				t.Fatalf("Text(%q) at column %d = (%q,%v), Put = (%q,%v)", text, i, c.ch, c.wide, gb.Cells[i].ch, gb.Cells[i].wide)
			}
		}
	})
}
