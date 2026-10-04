package decker

import (
	"bufio"
	"embed"
	"fmt"
	"io/fs"
	pathpkg "path"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// Block fonts: classic FIGlet fonts, designed as terminal characters
// (█ ▀ ╗ ═ …) and drawn onto the pixel canvas at a screen-relative scale.
// Use them for titles and impact lines; see Block for drawing and
// blockfx.go for animations. Font files and provenance are in fonts/figlet.

//go:embed fonts/figlet/*.flf
var figFiles embed.FS

// The stock block fonts. Load more FIGlet fonts with LoadFigFont.
var (
	BlockShadow = stockFig("ANSI Shadow")          // █ with box-drawing shadow; 6 rows
	BlockSolid  = stockFig("ANSI Regular")         // solid blocks; 5 rows
	BlockSmall  = stockFig("Calvin S")             // thin box-drawing; 3 rows
	BlockHuge   = stockFig("DOS Rebel")            // tall shaded blocks; 9 rows
	BlockFancy  = stockFig("Delta Corps Priest 1") // dramatic; 8 rows
)

// FigFont is a parsed FIGlet font.
type FigFont struct {
	Name    string
	Height  int
	kerning bool // false = full width (each glyph keeps its own box)
	glyphs  map[rune][][]rune
}

func stockFig(name string) *FigFont { return LoadFigFont(figFiles, "fonts/figlet/"+name+".flf") }

// LoadFigFont loads a FIGlet (.flf) font from fsys, named after its file.
// Fonts whose glyphs are made of block (█ ▀ ▄ ░) and box-drawing (═ ║ ╗)
// characters draw best. It panics if the font is missing or broken.
func LoadFigFont(fsys fs.FS, path string) *FigFont {
	b, err := fs.ReadFile(fsys, path)
	if err != nil {
		panic(err)
	}
	name := strings.TrimSuffix(pathpkg.Base(path), ".flf")
	f, err := parseFig(name, string(b))
	if err != nil {
		panic(path + ": " + err.Error())
	}
	return f
}

// parseFig reads the FIGlet .flf format: a header line, comment lines, then
// each character's rows. Rows end with an end mark (usually '@', doubled on
// a character's last row). Only the required ASCII characters are loaded.
func parseFig(name, src string) (*FigFont, error) {
	sc := bufio.NewScanner(strings.NewReader(src))
	sc.Buffer(make([]byte, 1<<16), 1<<20)
	if !sc.Scan() {
		return nil, fmt.Errorf("empty font")
	}
	hdr := strings.Fields(sc.Text())
	if len(hdr) < 6 || !strings.HasPrefix(hdr[0], "flf2a") {
		return nil, fmt.Errorf("not a FIGlet font")
	}
	hardblank, _ := utf8.DecodeRuneInString(hdr[0][5:])
	height, _ := strconv.Atoi(hdr[1])
	oldLayout, _ := strconv.Atoi(hdr[4])
	comments, _ := strconv.Atoi(hdr[5])
	for i := 0; i < comments && sc.Scan(); i++ {
	}
	f := &FigFont{Name: name, Height: height, kerning: oldLayout >= 0, glyphs: map[rune][][]rune{}}
	for r := rune(32); r <= 126; r++ {
		rows := make([][]rune, 0, height)
		for i := 0; i < height; i++ {
			if !sc.Scan() {
				return nil, fmt.Errorf("truncated at %q", r)
			}
			line := strings.TrimRight(sc.Text(), " \r")
			if line != "" {
				end := line[len(line)-1:]
				line = strings.TrimRight(line, end) // strip the end mark(s)
			}
			row := []rune(line)
			for j, ch := range row {
				if ch == hardblank {
					row[j] = '\u00a0' // placeholder; becomes a space after layout
				}
			}
			rows = append(rows, row)
		}
		f.glyphs[r] = rows
	}
	return f, nil
}

func (f *FigFont) glyph(r rune) [][]rune {
	if g, ok := f.glyphs[r]; ok {
		return g
	}
	if g, ok := f.glyphs[toUpper(r)]; ok {
		return g
	}
	return f.glyphs['?']
}

func toUpper(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - 32
	}
	return r
}

// render lays out one line of text. It returns the rows and, for every
// cell, the index of the source character it came from (-1 for blanks).
// Layouts are remembered, since block text is measured and drawn every
// frame; callers must not modify the result.
func (f *FigFont) render(s string) ([][]rune, [][]int) {
	key := figKey{f, s}
	figMu.Lock()
	l, ok := figCache[key]
	figMu.Unlock()
	if !ok {
		l.rows, l.owner = f.layout(s)
		figMu.Lock()
		if len(figCache) > 4096 {
			clear(figCache)
		}
		figCache[key] = l
		figMu.Unlock()
	}
	return l.rows, l.owner
}

type figKey struct {
	f *FigFont
	s string
}

type figLayout struct {
	rows  [][]rune
	owner [][]int
}

var (
	figMu    sync.Mutex
	figCache = map[figKey]figLayout{}
)

func (f *FigFont) layout(s string) ([][]rune, [][]int) {
	rows := make([][]rune, f.Height)
	owner := make([][]int, f.Height)
	for i, r := range []rune(s) {
		g := f.glyph(r)
		gw := 0
		for _, gr := range g {
			gw = max(gw, len(gr))
		}
		// Kerning: slide the glyph left until it would touch what's there.
		shift := 0
		if f.kerning && i > 0 {
			shift = 1 << 30
			for y := 0; y < f.Height; y++ {
				trail := 0
				for x := len(rows[y]) - 1; x >= 0 && rows[y][x] == ' '; x-- {
					trail++
				}
				lead := 0
				for x := 0; x < len(g[y]) && g[y][x] == ' '; x++ {
					lead++
				}
				if len(g[y]) == 0 {
					lead = gw
				}
				shift = min(shift, trail+lead)
			}
			if shift == 1<<30 {
				shift = 0
			}
		}
		for y := 0; y < f.Height; y++ {
			start := len(rows[y]) - shift
			for x := 0; x < gw; x++ {
				ch := ' '
				if x < len(g[y]) {
					ch = g[y][x]
				}
				pos := start + x
				for pos >= len(rows[y]) {
					rows[y] = append(rows[y], ' ')
					owner[y] = append(owner[y], -1)
				}
				if ch != ' ' {
					rows[y][pos] = ch
					owner[y][pos] = i
				}
			}
		}
	}
	for y := range rows {
		for x, ch := range rows[y] {
			if ch == '\u00a0' {
				rows[y][x] = ' '
				owner[y][x] = -1
			}
		}
	}
	return trimBlankRows(rows, owner)
}

// trimBlankRows removes empty rows at the top and bottom of a glyph block
// (many fonts pad with an empty row), so blocks stack tightly.
func trimBlankRows(rows [][]rune, owner [][]int) ([][]rune, [][]int) {
	blank := func(r []rune) bool { return strings.TrimSpace(string(r)) == "" }
	for len(rows) > 0 && blank(rows[len(rows)-1]) {
		rows, owner = rows[:len(rows)-1], owner[:len(owner)-1]
	}
	for len(rows) > 0 && blank(rows[0]) {
		rows, owner = rows[1:], owner[1:]
	}
	return rows, owner
}

// Width returns the width in cells of s rendered on one line.
func (f *FigFont) Width(s string) int {
	rows, _ := f.render(s)
	w := 0
	for _, r := range rows {
		w = max(w, len(r))
	}
	return w
}

// Rows returns the height in cells of one rendered line.
func (f *FigFont) Rows() int {
	rows, _ := f.render("AgjM")
	return len(rows)
}

// Wrap breaks s into lines no wider than maxW cells, at spaces. "\n" is kept.
func (f *FigFont) Wrap(s string, maxW int) []string {
	var out []string
	for _, para := range strings.Split(s, "\n") {
		line := ""
		for _, word := range strings.Fields(para) {
			try := word
			if line != "" {
				try = line + " " + word
			}
			if line != "" && f.Width(try) > maxW {
				out = append(out, line)
				line = word
			} else {
				line = try
			}
		}
		out = append(out, line)
	}
	return out
}

// Has reports whether the font has a visible glyph for every non-space
// character in s (some fonts have no digits or punctuation).
func (f *FigFont) Has(s string) bool {
	for _, r := range s {
		if r == ' ' || r == '\n' {
			continue
		}
		g, ok := f.glyphs[r]
		if !ok {
			g, ok = f.glyphs[toUpper(r)]
		}
		if !ok {
			return false
		}
		ink := false
		for _, row := range g {
			if strings.TrimSpace(strings.ReplaceAll(string(row), "\u00a0", " ")) != "" {
				ink = true
			}
		}
		if !ink {
			return false
		}
	}
	return true
}

// DropQuotes removes quote marks the font has no glyph for. Big block
// letters read fine without them ("AGENTS ARENT USERS"), and dropping them
// beats falling back to a smaller font. FitBlock does this itself; use it
// when you wrap or measure a string FitBlock fitted.
func (f *FigFont) DropQuotes(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\'', '’', '‘', '"', '“', '”', '`':
			if !f.Has(string(r)) {
				return -1
			}
		}
		return r
	}, s)
}
