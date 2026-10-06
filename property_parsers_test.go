package decker

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func escapedMarkup(s string) string {
	return strings.NewReplacer("\\", "\\\\", "*", "\\*", "_", "\\_", "`", "\\`", "{", "\\{", "}", "\\}").Replace(s)
}

func spanText(spans []Span) string {
	var b strings.Builder
	for _, span := range spans {
		b.WriteString(span.Text)
	}
	return b.String()
}

func FuzzMarkupRoundTrip(f *testing.F) {
	for _, s := range []string{"", "*bold* _muted_ `code`", "{accent:a {u:b}}", "\\*{}\\", "界\ne\u0301", "\x00\xff"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 4096 {
			return
		}
		escaped := escapedMarkup(s)
		for _, text := range []string{escaped, "{accent:" + escaped + "}", "{muted:{u:" + escaped + "}}"} {
			if got := spanText(ParseSpans(text, testTheme)); got != s {
				t.Fatalf("ParseSpans(%q) lost literal text: %q, want %q", text, got, s)
			}
		}
		// Inner color tags override an outer color for every emitted span.
		for _, span := range ParseSpans("{warn:{good:"+escaped+"}}", testTheme) {
			if span.Color == nil || *span.Color != testTheme.Good {
				t.Fatal("inner tag did not win")
			}
		}
	})
}

func simpleFiglet(height int, hardblank, end rune) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "flf2a%c %d 1 2 -1 0\n", hardblank, height)
	for r := rune(32); r <= 126; r++ {
		for row := range height {
			fmt.Fprintf(&b, "█%c%c", hardblank, end)
			if row == height-1 {
				b.WriteRune(end)
			}
			b.WriteByte('\n')
		}
	}
	return []byte(b.String())
}

func FuzzFigletParser(f *testing.F) {
	for _, b := range [][]byte{nil, []byte("flf2a$ 0 0 0 0 0"), []byte("flf2a$ 1 1 1 0 -1"), simpleFiglet(1, '$', '@'), simpleFiglet(2, '$', '界')} {
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64*1024 {
			return
		}
		font, err := ParseFigletFont("property", data)
		if err != nil {
			if font != nil {
				t.Fatal("parser returned a partial font with an error")
			}
			return
		}
		if font == nil || font.Name != "property" || font.Height <= 0 || len(font.glyphs) != 95 {
			t.Fatal("parser returned an incomplete font")
		}
		for r := rune(32); r <= 126; r++ {
			if len(font.glyphs[r]) != font.Height {
				t.Fatal("glyph row count disagrees with header")
			}
		}
		if font.Width("AgjM") < 0 || font.Rows() < 0 || font.Rows() > font.Height {
			t.Fatal("parsed font has invalid dimensions")
		}
	})
}

func FuzzFigletEndmark(f *testing.F) {
	for _, r := range []rune{'@', '#', '界', '§', '🎉'} {
		f.Add(int32(r), uint8(1))
	}
	f.Fuzz(func(t *testing.T, mark int32, hb uint8) {
		r := rune(mark)
		if !utf8.ValidRune(r) || r <= ' ' || r == '$' || r == '█' || r == utf8.RuneError {
			return
		}
		height := int(hb%4) + 1
		font, err := ParseFigletFont("endmark", simpleFiglet(height, '$', r))
		if err != nil {
			t.Fatal(err)
		}
		for ch := rune(32); ch <= 126; ch++ {
			for _, row := range font.glyphs[ch] {
				if string(row) != "█"+string(hardBlank) {
					t.Fatalf("endmark %q remained in glyph %q: %q", r, ch, string(row))
				}
			}
		}
	})
}
