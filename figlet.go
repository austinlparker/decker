package decker

import (
	"bufio"
	"embed"
	"fmt"
	"io/fs"
	"math"
	pathpkg "path"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

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

// FigFont is a parsed FIGlet (.flf) font: classic terminal-character type
// (█ ▀ ╗ ═ …) that Block draws onto the pixel canvas at a screen-relative
// scale, for titles and impact lines. Load more with LoadFigFont; the stock
// fonts' files and provenance are in fonts/figlet.
type FigFont struct {
	Name string // the font's file name without .flf
	// Height is the row count the font file declares. Rendered lines trim
	// blank padding rows, so use Rows for the height of drawn text.
	Height  int
	kerning bool // false = full width (each glyph keeps its own box)
	glyphs  map[rune][][]rune
}

// hardBlank stands in for the font's hardblank character, which keeps
// glyphs from kerning into each other, until layout turns it into a space.
const hardBlank = ' '

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
		for range height {
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
					row[j] = hardBlank
				}
			}
			rows = append(rows, row)
		}
		f.glyphs[r] = rows
	}
	return f, nil
}

// lookup returns r's glyph, falling back to its uppercase.
func (f *FigFont) lookup(r rune) ([][]rune, bool) {
	if g, ok := f.glyphs[r]; ok {
		return g, true
	}
	g, ok := f.glyphs[toUpper(r)]
	return g, ok
}

func (f *FigFont) glyph(r rune) [][]rune {
	if g, ok := f.lookup(r); ok {
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

// figCell is one cell of a rendered line: its character, and the index of
// the source character it came from (-1 for blanks).
type figCell struct {
	r     rune
	owner int
}

type figLayout [][]figCell

func (l figLayout) width() int {
	w := 0
	for _, row := range l {
		w = max(w, len(row))
	}
	return w
}

type figKey struct {
	f *FigFont
	s string
}

// Block text is measured and drawn every frame.
var figLayouts = memo[figKey, figLayout]{max: 4096}

// render lays out one line of text. Callers must not modify the result.
func (f *FigFont) render(s string) figLayout {
	return figLayouts.get(figKey{f, s}, func() figLayout { return f.layout(s) })
}

func (f *FigFont) layout(s string) figLayout {
	rows := make(figLayout, f.Height)
	for i, r := range []rune(s) {
		g := f.glyph(r)
		gw := 0
		for _, gr := range g {
			gw = max(gw, len(gr))
		}
		shift := 0
		if f.kerning && i > 0 {
			shift = kernShift(rows, g, gw)
		}
		for y := range f.Height {
			start := len(rows[y]) - shift
			for x := range gw {
				ch := ' '
				if x < len(g[y]) {
					ch = g[y][x]
				}
				for start+x >= len(rows[y]) {
					rows[y] = append(rows[y], figCell{' ', -1})
				}
				if ch != ' ' {
					rows[y][start+x] = figCell{ch, i}
				}
			}
		}
	}
	for _, row := range rows {
		for x, c := range row {
			if c.r == hardBlank {
				row[x] = figCell{' ', -1}
			}
		}
	}
	return trimBlankRows(rows)
}

// kernShift is how far glyph g (gw wide) can slide left into rows before it
// would touch what's there.
func kernShift(rows figLayout, g [][]rune, gw int) int {
	shift := math.MaxInt
	for y, row := range rows {
		trail := 0
		for x := len(row) - 1; x >= 0 && row[x].r == ' '; x-- {
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
	if shift == math.MaxInt {
		return 0
	}
	return shift
}

// trimBlankRows removes empty rows at the top and bottom of a glyph block
// (many fonts pad with an empty row), so blocks stack tightly.
func trimBlankRows(rows figLayout) figLayout {
	blank := func(row []figCell) bool {
		return !slices.ContainsFunc(row, func(c figCell) bool { return !unicode.IsSpace(c.r) })
	}
	for len(rows) > 0 && blank(rows[len(rows)-1]) {
		rows = rows[:len(rows)-1]
	}
	for len(rows) > 0 && blank(rows[0]) {
		rows = rows[1:]
	}
	return rows
}

// Width returns the width in cells of s rendered on one line.
func (f *FigFont) Width(s string) int { return f.render(s).width() }

// widest returns the width in cells of the widest of lines, each rendered on
// one line.
func (f *FigFont) widest(lines []string) int {
	w := 0
	for _, l := range lines {
		w = max(w, f.Width(l))
	}
	return w
}

// Rows returns the height in cells of one rendered line, blank padding rows
// trimmed. It is measured on a sample with ascenders, descenders and capitals
// so that it is the same for every line of text.
func (f *FigFont) Rows() int { return len(f.render("AgjM")) }

// Wrap breaks s greedily into lines no wider than maxW cells, at spaces,
// keeping "\n". Unlike Font.Wrap it does not balance the lines.
func (f *FigFont) Wrap(s string, maxW int) []string {
	return wrapGreedy(s, float64(maxW), func(l string) float64 { return float64(f.Width(l)) })
}

// Has reports whether the font has a visible glyph for every non-space
// character in s (some fonts have no digits or punctuation).
func (f *FigFont) Has(s string) bool {
	for _, r := range s {
		if r == ' ' || r == '\n' {
			continue
		}
		g, ok := f.lookup(r)
		if !ok || !slices.ContainsFunc(g, func(row []rune) bool {
			return slices.ContainsFunc(row, func(r rune) bool { return !unicode.IsSpace(r) })
		}) {
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
